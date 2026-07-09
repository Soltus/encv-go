package server

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed simverse_web/*
var simverseWebFS embed.FS

func getSimverseWebSubFS() fs.FS {
	subFS, err := fs.Sub(simverseWebFS, "simverse_web")
	if err != nil {
		panic(err)
	}
	return subFS
}

func registerSimverseWebRoutes(r *gin.Engine) {
	subFS := getSimverseWebSubFS()
	fileServer := http.FileServer(http.FS(subFS))

	r.GET("/simverse-home", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/simverse/")
	})

	r.GET("/simverse-home/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/simverse/")
	})

	simverseGroup := r.Group("/simverse")
	{
		simverseGroup.GET("/*filepath", func(c *gin.Context) {
			filepath := c.Param("filepath")
			if filepath == "" || filepath == "/" {
				c.Request.URL.Path = "/"
			} else {
				c.Request.URL.Path = filepath
			}
			fileServer.ServeHTTP(c.Writer, c.Request)
		})
	}
}
