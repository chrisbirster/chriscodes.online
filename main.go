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
	"strconv"
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

func (tr *TemplateRenderer) Render(w http.ResponseWriter, tmpl string, data interface{}, ctx context.Context) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, err := template.ParseFiles(path.Join(tr.templatePath, tmpl))
	if err != nil {
		return err
	}

	return t.Execute(w, data)
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
	logger       *slog.Logger
	IndexHandler *IndexHandler
	UserHandler  *UserHandler
	renderer     *TemplateRenderer
}

func (A *App) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	var head string
	head, req.URL.Path = ShiftPath(req.URL.Path)

	switch head {
	case "":
		A.IndexHandler.ServeHTTP(res, req)
		return
	case "user":
		A.UserHandler.ServeHTTP(res, req)
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
		I.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path), slog.String("location", "user list route"))
	}

	data := struct {
		Title string
	}{
		Title: "Index Page",
	}

	switch req.Method {
	case "GET":
		I.renderer.Render(res, "index.html", data, req.Context())
		return
	default:
		I.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path))
		http.Error(res, "Only GET is supported", http.StatusMethodNotAllowed)
	}
}

type UserHandler struct {
	logger *slog.Logger
}

func (U *UserHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	var head string
	head, req.URL.Path = ShiftPath(req.URL.Path)

	if head == "" {
		U.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path), slog.String("location", "user list route"))
		fmt.Fprintf(res, "User list")
		return
	}

	id, err := strconv.Atoi(head)
	if err != nil {
		U.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path))
		http.Error(res, fmt.Sprintf("Invalid user id %q", head), http.StatusBadRequest)
		return
	}

	switch req.Method {
	case "GET":
		U.getUser(res, req, id)
	default:
		U.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path))
		http.Error(res, "Only GET is supported", http.StatusMethodNotAllowed)
	}
}

func (U UserHandler) getUser(res http.ResponseWriter, req *http.Request, id int) {
	U.logger.Info("request", slog.String("method", req.Method), slog.String("path", req.URL.Path))
	fmt.Fprintf(res, "User %d", id)

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
	userHandler := &UserHandler{
		logger,
	}

	app := &App{
		logger:       logger,
		IndexHandler: indexHandler,
		UserHandler:  userHandler,
		renderer:     renderer,
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
