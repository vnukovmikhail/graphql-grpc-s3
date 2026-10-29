package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"

	firebase "firebase.google.com/go"
	"google.golang.org/api/option"
)

const FILES = "./files/"

type File struct {
	ID          string    `json:"id" pg:",pk,type:uuid"`
	UserID      string    `json:"user_id" pg:",notnull"`
	CreatedAt   time.Time `json:"created_at" pg:"default:now()"`
	Size        int64     `json:"size"`
	Filename    string    `json:"name"`
	ContentType string    `json:"content_type"`
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
	app, err := firebase.NewApp(context.Background(), nil, option.WithCredentialsFile("private_keys.json"))
	if err != nil {
		panic(err)
	}

	client, err := app.Auth(context.Background())
	if err != nil {
		panic(err)
	}

	r := gin.Default()
	db := pg.Connect(&pg.Options{
		User:     "postgres",
		Password: "postgres",
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

		ctx.Header("Content-Type", file.ContentType)
		ctx.Header("Content-Disposition", "filename="+quoteEscaper.Replace(file.Filename))
		ctx.File(filepath.Join(FILES, file.ID))
	})

	r.POST("/files", func(ctx *gin.Context) {
		authToken, err := client.VerifyIDToken(context.Background(), ctx.Request.FormValue("token"))
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
		localFile, err := os.Create(filepath.Join(FILES, id))
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		_, err = io.Copy(localFile, userFile)
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}

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
		}

		_, err = db.Model(f).Insert()
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		ctx.JSON(http.StatusOK, f)
	})

	r.Run()
}
