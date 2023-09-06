package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LIGMA - A lightweight intuitive generator for markdown applications
type LIGMA struct {
	Content map[string]string
	Slugs   []string
}

func NewLIGMA() *LIGMA {
	return &LIGMA{
		Content: make(map[string]string),
		Slugs:   make([]string, 0),
	}
}

// GetBlogContent retrieves the content from the cache or loads and parses it
func (l *LIGMA) GetBlogContent(slug string) (string, error) {
	content, found := l.Content[slug]
	if found {
		return content, nil
	}

	parsedContent, err := l.loadBlogContent(slug)
	if err != nil {
		return "", err
	}

	l.Content[slug] = parsedContent
	return parsedContent, nil
}

// / buildBlogList builds a list of blog posts from the content directory
func (l *LIGMA) buildBlogList() error {
    return l.buildBlogListRecursive("content")
}

func (l *LIGMA) buildBlogListRecursive(dir string) error {
    markdownSlugs := make([]string, 0)

    files, err := os.ReadDir(dir)
    if err != nil {
        return err
    }

    for _, file := range files {
        path := filepath.Join(dir, file.Name())
        if file.IsDir() {
            // If it's a directory, scan its content recursively.
            err := l.buildBlogListRecursive(path)
            if err != nil {
                return err
            }
        } else if strings.HasSuffix(file.Name(), ".md") {
            slug := strings.TrimPrefix(path, "content/")
            slug = strings.TrimSuffix(slug, ".md")
            markdownSlugs = append(markdownSlugs, slug)
        }
    }

    l.Slugs = append(l.Slugs, markdownSlugs...)
    return nil
}

func (l *LIGMA) loadBlogContent(slug string) (string, error) {
	file, err := os.Open(fmt.Sprintf("content/%s.md", slug))
	if err != nil {
		return "", err
	}
	defer file.Close()

	var html strings.Builder

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Headers
		if strings.HasPrefix(line, "### ") {
			html.WriteString("<h3>" + line[4:] + "</h3>")
		} else if strings.HasPrefix(line, "## ") {
			html.WriteString("<h2>" + line[3:] + "</h2>")
		} else if strings.HasPrefix(line, "# ") {
			html.WriteString("<h1>" + line[2:] + "</h1>")
		} else if strings.HasPrefix(line, "- ") {
			html.WriteString("<ul><li>" + line[2:] + "</li></ul>")
		} else {
			html.WriteString(l.parseInlineMarkdown(line) + "<br>")
		}
	}

	return html.String(), scanner.Err()
}

func (l *LIGMA) parseInlineMarkdown(line string) string {
	var buf bytes.Buffer
	inStrong := false
	inEm := false
	inLinkText := false
	inLinkURL := false
	linkText := ""

	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == '*' && i+1 < len(line) && line[i+1] == '*':
			if inStrong {
				buf.WriteString("</strong>")
				inStrong = false
			} else {
				buf.WriteString("<strong>")
				inStrong = true
			}
			i++ // skip next "*"
		case line[i] == '*' && !inStrong:
			if inEm {
				buf.WriteString("</em>")
				inEm = false
			} else {
				buf.WriteString("<em>")
				inEm = true
			}
		case line[i] == '[':
			inLinkText = true
		case line[i] == ']':
			inLinkText = false
		case line[i] == '(' && inLinkText:
			inLinkURL = true
		case line[i] == ')' && inLinkURL:
			inLinkURL = false
			buf.WriteString(`<a href="` + linkText + `">` + linkText + "</a>")
			linkText = ""
		default:
			if inLinkURL {
				linkText += string(line[i])
			} else {
				buf.WriteByte(line[i])
			}
		}
	}
	return buf.String()
}
