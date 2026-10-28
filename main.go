package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"uuid"

	"github.com/davecgh/go-spew/spew"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
)

const FILES = "./files/"

type File struct {
	ID          string `json:"id" pg:",pk"`
	Size        int64  `json:"size"`
	Filename    string `json:"name"`
	ContentType string `json:"content_type"`
}

var (
	db           *pg.DB
	quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")
)

func main() {
	r := gin.Default()
	db := pg.Connect(&pg.Options{
		User:     "postgres",
		Password: "postgres",
	})
	defer db.Close()

	models := []interface{}{
		(*File)(nil),
	}

	for _, model := range models {
		err := db.Model(model).CreateTable(&orm.CreateTableOptions{
			IfNotExists: true,
		})
		if err != nil {
			panic(err)
		}
	}

	r.Use(static.Serve("/", static.LocalFile("static", false)))

	r.GET("/files/:id", func(ctx *gin.Context) {
		file := File{ID: ctx.Param("id")}

		err := db.Model(&file).WherePK().Select()
		if err != nil {
			ctx.AbortWithStatus(http.StatusNotFound)
			return
		}

		ctx.Header("Content-Type", file.ContentType)
		ctx.Header("Content-Disposition", "filename="+quoteEscaper.Replace(file.Filename))
		ctx.File(filepath.Join(FILES, file.ID))
	})

	r.POST("/files", func(ctx *gin.Context) {
		header, err := ctx.FormFile("file")
		if err != nil {
			ctx.AbortWithError(http.StatusBadRequest, err)
			return
		}

		spew.Dump(header.Header)

		userFile, err := header.Open()
		if err != nil {
			ctx.AbortWithError(http.StatusBadRequest, err)
			return
		}

		fmt.Println("copying " + header.Filename)

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
