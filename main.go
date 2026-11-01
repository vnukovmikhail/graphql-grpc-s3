package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
	"github.com/joho/godotenv"

	firebase "firebase.google.com/go"
	"google.golang.org/api/option"
)

const (
	FILES = "./files/"
	ITEMS = 20
)

type File struct {
	ID          string    `json:"id" pg:",pk,type:uuid"`
	UserID      string    `json:"user_id" pg:",notnull"`
	CreatedAt   time.Time `json:"created_at" pg:"default:now()"`
	Size        int64     `json:"size"`
	Filename    string    `json:"name"`
	ContentType string    `json:"content_type"`
	Location    string    `json:"location"`
}

var (
	db           *pg.DB
	quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")
)

type dbLogger struct{}

func (d dbLogger) BeforeQuery(ctx context.Context, q *pg.QueryEvent) (context.Context, error) {
	return ctx, nil
}

func (d dbLogger) AfterQuery(ctx context.Context, q *pg.QueryEvent) error {
	l, _ := q.FormattedQuery()
	fmt.Println(string(l))
	return nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	app, err := firebase.NewApp(context.Background(), nil, option.WithCredentialsFile("private_keys.json"))
	if err != nil {
		panic(err)
	}

	client, err := app.Auth(context.Background())
	if err != nil {
		panic(err)
	}

	sess := session.Must(session.NewSession())
	svc := s3.New(sess)
	uploader := s3manager.NewUploader(sess)

	r := gin.Default()
	db := pg.Connect(&pg.Options{
		User:     os.Getenv("POSTGRES_USERNAME"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
	})
	defer db.Close()
	db.AddQueryHook(dbLogger{})

	models := []interface{}{
		(*File)(nil),
	}

	for _, model := range models {
		db.Model(model).DropTable(&orm.DropTableOptions{IfExists: true})
		err := db.Model(model).CreateTable(&orm.CreateTableOptions{
			IfNotExists: true,
		})
		if err != nil {
			panic(err)
		}
	}

	if _, err := db.Exec(`CREATE INDEX idx_user_id ON files (user_id)`); err != nil {
		panic(err)
	}

	r.Use(static.Serve("/", static.LocalFile("static", false)))

	r.GET("/files", func(ctx *gin.Context) {
		p, err := strconv.Atoi(ctx.Query("page"))
		if err != nil {
			p = 0
		} else if p < 0 {
			p = 0
		}

		authToken, err := client.VerifyIDToken(context.Background(), ctx.GetHeader("Authorization"))
		if err != nil {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		files := make([]File, 0)
		if err = db.Model(&files).Where("user_id = ?", authToken.UID).Offset(p * ITEMS).Limit(ITEMS).Select(); err != nil {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		ctx.JSON(http.StatusOK, files)
	})

	r.GET("/files/:id", func(ctx *gin.Context) {
		paramID := ctx.Param("id")

		if _, err := uuid.Parse(paramID); err != nil {
			ctx.AbortWithStatus(http.StatusNotFound)
			return
		}

		file := File{ID: paramID}

		err = db.Model(&file).WherePK().Select()
		if err != nil {
			ctx.AbortWithStatus(http.StatusNotFound)
			return
		}

		// ctx.Header("Content-Type", file.ContentType)
		// ctx.Header("Content-Security-Policy", "default-src 'none'")
		// ctx.Header("Content-Disposition", "filename="+quoteEscaper.Replace(file.Filename))
		// ctx.File(filepath.Join(FILES, file.ID))
		output, err := svc.GetObject(&s3.GetObjectInput{
			Bucket: aws.String(os.Getenv("AWS_BUCKET")),
			Key:    aws.String(file.ID),
		})
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		fmt.Printf("file downloaded, %d bytes\n", output)

		ctx.DataFromReader(http.StatusOK, *output.ContentLength, file.ContentType, output.Body, map[string]string{
			"Content-Security-Policy": "default-src 'none'",
			"Content-Disposition":     "filename=" + quoteEscaper.Replace(file.Filename) + "\"",
		})
	})

	r.DELETE("/files/:id", func(ctx *gin.Context) {
		paramID := ctx.Param("id")

		if _, err := uuid.Parse(paramID); err != nil {
			ctx.AbortWithStatus(http.StatusNotFound)
			return
		}

		authToken, err := client.VerifyIDToken(context.Background(), ctx.GetHeader("Authorization"))
		if err != nil {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		file := File{ID: paramID}

		if err = db.Model(&file).WherePK().Select(); err != nil {
			ctx.AbortWithStatus(http.StatusNotFound)
			return
		}

		if file.UserID == authToken.UID {
			if _, err := db.Model(&file).WherePK().Delete(); err != nil {
				ctx.AbortWithStatus(http.StatusInternalServerError)
				return
			}

			ctx.Status(http.StatusNoContent)
			_, err = svc.DeleteObject(&s3.DeleteObjectInput{
				Bucket: aws.String(os.Getenv("AWS_BUCKET")),
				Key:    aws.String(file.ID),
			})
			if err != nil {
				ctx.AbortWithError(http.StatusInternalServerError, err)
				return
			}

		} else {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	})

	r.POST("/files", func(ctx *gin.Context) {
		authToken, err := client.VerifyIDToken(context.Background(), ctx.GetHeader("Authorization")) // LAST
		if err != nil {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		header, err := ctx.FormFile("file")
		if err != nil {
			ctx.AbortWithError(http.StatusBadRequest, err)
			return
		}

		userFile, err := header.Open()
		if err != nil {
			ctx.AbortWithError(http.StatusBadRequest, err)
			return
		}

		id := uuid.New().String()
		// localFile, err := os.Create(filepath.Join(FILES, id))
		// if err != nil {
		// 	ctx.AbortWithError(http.StatusInternalServerError, err)
		// 	return
		// }

		// _, err = io.Copy(localFile, userFile)
		// if err != nil {
		// 	ctx.AbortWithError(http.StatusInternalServerError, err)
		// 	return
		// }
		result, err := uploader.Upload(&s3manager.UploadInput{
			Bucket:             aws.String(os.Getenv("AWS_BUCKET")),
			Key:                aws.String(id),
			Body:               userFile,
			ContentType:        aws.String(header.Header.Get("Content-Type")),
			ContentDisposition: aws.String("filename" + quoteEscaper.Replace(header.Filename) + "\""),
		})
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		fmt.Printf("file uploaded to, %s\n", result.Location)

		s, err := url.QueryUnescape(header.Filename)
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		f := &File{
			ID:          id,
			UserID:      authToken.UID,
			Size:        header.Size,
			Filename:    s,
			ContentType: header.Header.Get("Content-Type"),
			Location:    result.Location,
		}

		_, err = db.Model(f).Insert()
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		ctx.JSON(http.StatusOK, f)
	})

	r.POST("/login", func(ctx *gin.Context) {
		var body struct {
			From string
			To   string
		}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.AbortWithStatus(http.StatusBadRequest)
			return
		}

		if body.From == "" {
			ctx.Status(http.StatusNoContent)
			return
		}

		fromAuthToken, err := client.VerifyIDToken(context.Background(), body.From)
		if err != nil || fromAuthToken.Firebase.SignInProvider != "anonymous" {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		toAuthToken, err := client.VerifyIDToken(context.Background(), body.To)
		if err != nil || toAuthToken.Firebase.SignInProvider == "anonymous" {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		_, err = db.Model(&File{}).Where("user_id = ?", fromAuthToken.UID).Set("user_id = ?", toAuthToken.UID).UpdateNotZero()
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		ctx.Status(http.StatusNoContent)
	})

	r.Run(":8080")
}
