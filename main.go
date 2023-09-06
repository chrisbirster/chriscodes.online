package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/fatih/color"
)

type TemplateRenderer struct {
	templatePath string
}

func NewTemplateRenderer(templatePath string) *TemplateRenderer {
	return &TemplateRenderer{
		templatePath: templatePath,
	}
}

func (tr *TemplateRenderer) Render(w http.ResponseWriter, page string, layout string, data interface{}, ctx context.Context) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Build the full path of the layout and the page.
	layoutFile := path.Join(tr.templatePath, "layouts", layout)
	pageFile := path.Join(tr.templatePath, "pages", page)

	t, err := template.ParseFiles(layoutFile, pageFile)
	if err != nil {
		return err
	}

	return t.ExecuteTemplate(w, "base", data)
}

type PrettyHandlerOptions struct {
	SlogOpts slog.HandlerOptions
}

type PrettyHandler struct {
	slog.Handler
	l *log.Logger
}

func (p *PrettyHandler) Handle(ctx context.Context, r slog.Record) error {
	level := r.Level.String() + ":"

	switch r.Level {
	case slog.LevelDebug:
		level = color.MagentaString(level)
	case slog.LevelInfo:
		level = color.BlueString(level)
	case slog.LevelWarn:
		level = color.YellowString(level)
	case slog.LevelError:
		level = color.RedString(level)
	}

	fields := make(map[string]interface{}, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()

		return true
	})

	b, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}

	timeStr := r.Time.Format("[15:05:05.000]")
	msg := color.CyanString(r.Message)

	p.l.Println(timeStr, level, msg, color.WhiteString(string(b)))

	return nil
}

func NewPrettyHandler(out io.Writer, opts PrettyHandlerOptions) *PrettyHandler {
	p := &PrettyHandler{
		Handler: slog.NewJSONHandler(out, &opts.SlogOpts),
		l:       log.New(out, "", 0),
	}
	return p
}

type App struct {
	logger         *slog.Logger
	IndexHandler   *IndexHandler
	AboutHandler   *AboutHandler
	BlogHandler    *BlogHandler
	ContactHandler *ContactHandler
	renderer       *TemplateRenderer
}

func (A *App) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	var head string
	head, req.URL.Path = ShiftPath(req.URL.Path)

	switch head {
	case "":
		A.IndexHandler.ServeHTTP(res, req)
		return
	case "about":
		A.AboutHandler.ServeHTTP(res, req)
		return
	case "blog":
		A.BlogHandler.ServeHTTP(res, req)
		return
	case "contact":
		A.ContactHandler.ServeHTTP(res, req)
		return
	default:
		A.logger.Error("request", slog.String("method", req.Method), slog.String("path", req.URL.Path), slog.String("location", "index route"))
	}

	A.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path), slog.String("location", "index route"))

	http.Error(res, "Not Found", http.StatusNotFound)

}

type IndexHandler struct {
	logger   *slog.Logger
	renderer *TemplateRenderer
}

func (I *IndexHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	var head string
	head, req.URL.Path = ShiftPath(req.URL.Path)

	if head == "" {
		I.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path), slog.String("location", "index route"))
	}

	data := struct {
		Title string
	}{
		Title: "Index Page",
	}

	switch req.Method {
	case "GET":
		err := I.renderer.Render(res, "index.html", "base.html", data, req.Context())
		if err != nil {
			I.logger.Error("Render error", slog.String("error", err.Error()))
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	default:
		I.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path))
		http.Error(res, "Only GET is supported", http.StatusMethodNotAllowed)
	}
}

type AboutHandler struct {
	logger   *slog.Logger
	renderer *TemplateRenderer
}

func (A *AboutHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	var head string
	head, req.URL.Path = ShiftPath(req.URL.Path)

	if head == "" {
		A.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path), slog.String("location", "user list route"))
	}

	data := struct {
		Title string
	}{
		Title: "About Page",
	}

	switch req.Method {
	case "GET":
		A.renderer.Render(res, "about.html", "base.html", data, req.Context())
		return
	default:
		A.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path))
		http.Error(res, "Only GET is supported", http.StatusMethodNotAllowed)
	}
}

type BlogHandler struct {
	logger   *slog.Logger
	renderer *TemplateRenderer
	parser   *LIGMA
}

func (B *BlogHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	var head string
	head, req.URL.Path = ShiftPath(req.URL.Path)

    err := B.parser.buildBlogList()
    if err != nil {
        B.logger.Error("buildBlogList error", slog.String("error", err.Error()))
    }

	blogdata := struct {
		Title    string
		BlogList []string
	}{
		Title:    "Blog Page",
		BlogList: B.parser.Slugs,
	}

	if head == "" {
		B.logger.Info(
			"request",
			slog.String("method", req.Method),
			slog.String("path", req.URL.Path),
			slog.String("location", "blog list route"),
			slog.String("bloglist", strings.Join(B.parser.Slugs, ", ")),
		)

		switch req.Method {
		case "GET":
			B.renderer.Render(res, "blog.html", "base.html", blogdata, req.Context())
			return
		default:
			B.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path))
			http.Error(res, "Only GET is supported", http.StatusMethodNotAllowed)
		}
	}

	content, err := B.parser.loadBlogContent(head)
	if err != nil {
		B.logger.Error("loadBlogContent error", slog.String("error", err.Error()))
		http.Error(res, "blog content not found", http.StatusNotFound)
	}

	data := struct {
		Title   string
		Content string
	}{
		Title:   "Blog Page",
		Content: content,
	}

	B.renderer.Render(res, "blog.html", "base.html", data, req.Context())
}

type ContactHandler struct {
	logger   *slog.Logger
	renderer *TemplateRenderer
}

func (C *ContactHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	var head string
	head, req.URL.Path = ShiftPath(req.URL.Path)

	if head == "" {
		C.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path), slog.String("location", "user list route"))
		fmt.Fprintf(res, "User list")
		return
	}

	data := struct {
		Title string
	}{
		Title: "Blog Page",
	}

	switch req.Method {
	case "GET":
		C.renderer.Render(res, "blog.html", "base.html", data, req.Context())
		return
	default:
		C.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path))
		http.Error(res, "Only GET is supported", http.StatusMethodNotAllowed)
	}
}

func main() {
	opts := PrettyHandlerOptions{
		SlogOpts: slog.HandlerOptions{
			Level: slog.LevelDebug,
		},
	}

	handler := NewPrettyHandler(os.Stdout, opts)
	logger := slog.New(handler)
	renderer := NewTemplateRenderer("templates")
	indexHandler := &IndexHandler{
		logger,
		renderer,
	}
	aboutHandler := &AboutHandler{
		logger,
		renderer,
	}
	ligma := NewLIGMA()
	blogHandler := &BlogHandler{
		logger,
		renderer,
		ligma,
	}
	contactHandler := &ContactHandler{
		logger,
		renderer,
	}

	app := &App{
		logger:         logger,
		IndexHandler:   indexHandler,
		AboutHandler:   aboutHandler,
		BlogHandler:    blogHandler,
		ContactHandler: contactHandler,
		renderer:       renderer,
	}

	http.ListenAndServe(":8000", app)
}

// ShiftPath splits off the first component of p, which will be cleaned of
// relative components before processing. Head will never contain a slash and
// tail will always be a rooted path without a trailing slash.
func ShiftPath(p string) (head, tail string) {
	p = path.Clean("/" + p)
	i := strings.Index(p[1:], "/") + 1
	if i <= 0 {
		return p[1:], "/"
	}

	return p[1:i], p[i:]
}
