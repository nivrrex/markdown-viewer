package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

const tempNameLimit = 48

// exportPDF 在后台生成打印视图，再交给系统浏览器打印成 PDF。
// 原生 GPU 渲染窗口没有 PDF 输出接口，因此走系统打印路径，不引入额外依赖。
func (a *app) exportPDF(c *ui.Context) {
	if a.src == "" {
		c.Toast("先打开一个 Markdown 文件")
		return
	}
	if a.exporting {
		c.Toast("正在生成打印视图")
		return
	}
	a.exporting = true
	a.showNotice("正在生成打印视图…")
	c.Invalidate()

	path, src, win := a.path, a.src, a.win
	fail := func(message string) {
		win.Update(func() {
			a.exporting = false
			a.showError(message)
		})
	}
	go func() {
		page, err := renderPrintHTML(filepath.Base(path), path, src)
		if err != nil {
			fail("生成打印视图失败：" + err.Error())
			return
		}
		file, err := writeTempHTML(path, page)
		if err != nil {
			fail("写入打印视图失败：" + err.Error())
			return
		}
		if err := openInBrowser(file); err != nil {
			fail("无法打开浏览器：" + err.Error())
			return
		}
		win.Update(func() {
			a.exporting = false
			a.showNotice("已在浏览器打开打印视图，请用浏览器“打印 → 另存为 PDF”")
		})
	}()
}

// renderPrintHTML 把 Markdown 转成打印 HTML，并用原文件目录解析相对图片与链接。
// 原始 HTML 不传给浏览器（goldmark 默认不渲染），避免嵌入脚本被执行。
func renderPrintHTML(title, sourcePath, src string) (string, error) {
	var base template.URL
	if sourcePath != "" {
		abs, err := filepath.Abs(sourcePath)
		if err != nil {
			return "", fmt.Errorf("解析 Markdown 路径: %w", err)
		}
		base = directoryBaseURL(filepath.Dir(abs), runtime.GOOS == "windows")
	}
	var body bytes.Buffer
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithXHTML()),
	)
	if err := md.Convert([]byte(src), &body); err != nil {
		return "", err
	}
	data := struct {
		Title string
		Base  template.URL
		Body  template.HTML
	}{Title: title, Base: base, Body: template.HTML(body.String())}
	var out bytes.Buffer
	if err := printTemplate.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func directoryBaseURL(dir string, windows bool) template.URL {
	path := strings.TrimSuffix(filepath.ToSlash(dir), "/")
	if windows {
		path = strings.ReplaceAll(path, `\`, "/")
	}
	u := url.URL{Scheme: "file"}
	switch {
	case windows && strings.HasPrefix(path, "//"):
		host, rest, _ := strings.Cut(strings.TrimPrefix(path, "//"), "/")
		u.Host = host
		if rest == "" {
			u.Path = "/"
		} else {
			u.Path = "/" + rest + "/"
		}
	case windows && len(path) >= 2 && path[1] == ':':
		u.Path = "/" + path + "/"
	default:
		u.Path = path + "/"
	}
	return template.URL(u.String())
}

// writeTempHTML 把页面写到临时文件，文件名保留原文件的可读部分。
func writeTempHTML(sourcePath, page string) (string, error) {
	base := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	if base == "" {
		base = "markdown"
	}
	base = sanitizeName(base)
	f, err := os.CreateTemp("", base+"-*.html")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(page); err != nil {
		closeErr := f.Close()
		removeErr := os.Remove(f.Name())
		return "", errors.Join(err, closeErr, removeErr)
	}
	if err := f.Close(); err != nil {
		return "", errors.Join(err, os.Remove(f.Name()))
	}
	return f.Name(), nil
}

// sanitizeName 去掉文件名里可能干扰临时文件命名的字符，并限制长度。
func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r > 0x7f:
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := []rune(b.String())
	if len(out) > tempNameLimit {
		out = out[:tempNameLimit]
	}
	if len(out) == 0 {
		return "markdown"
	}
	return string(out)
}

// openInBrowser 用系统默认程序打开 HTML 文件。
func openInBrowser(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", cmd.Path, err)
	}
	return nil
}

// printTemplate 是打印视图的外壳：A4 页面、中文字体栈、代码块与表格样式。
var printTemplate = template.Must(template.New("print").Parse(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
{{if .Base}}<base href="{{.Base}}">{{end}}
<title>{{.Title}}</title>
<style>
  @page { size: A4; margin: 18mm 16mm; }
  :root { color-scheme: light; }
  body {
    margin: 0 auto;
    max-width: 190mm;
    font-family: "Noto Sans CJK SC", "Source Han Sans SC", "Microsoft YaHei", "PingFang SC", -apple-system, "Segoe UI", sans-serif;
    font-size: 11pt;
    line-height: 1.7;
    color: #1f2328;
    background: #fff;
  }
  h1, h2, h3, h4, h5, h6 { line-height: 1.3; margin: 1.4em 0 0.6em; }
  h1 { font-size: 1.7em; border-bottom: 1px solid #d0d7de; padding-bottom: 0.3em; }
  h2 { font-size: 1.4em; border-bottom: 1px solid #e6e9ee; padding-bottom: 0.25em; }
  h3 { font-size: 1.2em; }
  p, ul, ol, blockquote, table, pre { margin: 0 0 0.9em; }
  a { color: #0969da; text-decoration: none; }
  code {
    font-family: "JetBrains Mono", "Cascadia Mono", Consolas, "DejaVu Sans Mono", monospace;
    font-size: 0.9em;
    background: #f2f4f7;
    padding: 0.1em 0.35em;
    border-radius: 4px;
  }
  pre {
    background: #f6f8fa;
    border: 1px solid #d0d7de;
    border-radius: 6px;
    padding: 10px 12px;
    overflow-x: auto;
    white-space: pre-wrap;
    word-break: break-word;
  }
  pre code { background: none; padding: 0; font-size: 0.88em; }
  blockquote {
    margin-left: 0;
    padding: 0.2em 0 0.2em 1em;
    border-left: 4px solid #d0d7de;
    color: #59636e;
  }
  table { border-collapse: collapse; width: 100%; }
  th, td { border: 1px solid #d0d7de; padding: 6px 10px; text-align: left; }
  th { background: #f6f8fa; }
  img { max-width: 100%; }
  hr { border: none; border-top: 1px solid #d0d7de; margin: 1.5em 0; }
  @media print {
    body { max-width: none; }
    a { color: inherit; }
    pre, table, blockquote, img { break-inside: avoid; }
    h1, h2, h3, h4 { break-after: avoid; }
  }
</style>
</head>
<body>
{{.Body}}
</body>
</html>
`))
