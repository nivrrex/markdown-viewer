package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

const sampleMarkdown = "# 标题一\n\n正文段落，包含**粗体**、`行内代码`与[链接](https://example.com)。\n\n" +
	"- 列表项一\n- 列表项二\n\n" +
	"| 名称 | 数值 |\n|---|--:|\n| 甲 | 1 |\n| 乙 | 2 |\n\n" +
	"```go\nfunc main() {\n\tfmt.Println(\"你好\")\n}\n```\n"

func newTestApp(src string) *app {
	a := &app{fontSel: defaultFontLabel, codeSel: defaultFontLabel, zoom: defaultZoom, fontSize: defaultFontSizeForOS()}
	a.path = "/tmp/sample.md"
	a.src = src
	return a
}

// 阶段 1 验收：标题、段落、列表、表格都渲染出来。
func TestViewRendersMarkdown(t *testing.T) {
	tt := ui.NewTester(newTestApp(sampleMarkdown).view, 900, 700)
	for _, want := range []string{"标题一", "正文段落", "列表项一", "列表项二", "甲", "乙"} {
		if !tt.HasText(want) {
			t.Errorf("缺少文本 %q，实际 %q", want, tt.Texts())
		}
	}
}

// 空状态不崩溃。
func TestEmptyState(t *testing.T) {
	tt := ui.NewTester(newTestApp("").view, 900, 700)
	if !tt.HasText("打开") {
		t.Errorf("空状态缺少提示，实际 %q", tt.Texts())
	}
}

// 阶段 1 验收：5000 行以上滚动到底不重排、不卡顿。
func TestLargeDocumentScroll(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 5200; i++ {
		fmt.Fprintf(&b, "## 第 %d 节\n\n这是第 %d 节的内容，带**强调**与`代码`。\n\n", i, i)
	}
	a := newTestApp(b.String())

	start := time.Now()
	tt := ui.NewTester(a.view, 900, 700)
	first := time.Since(start)

	start = time.Now()
	a.scroll.Y = math.MaxFloat32
	tt.Frame()
	last := time.Since(start)

	if tt.FramePending() {
		t.Error("滚动到底部后视图仍在请求帧")
	}
	if !tt.HasText("第 5199 节") {
		t.Errorf("文档底部未渲染，实际尾部 %q", tail(tt.Texts(), 5))
	}
	t.Logf("5000+ 行：首帧 %v，滚动到底 %v", first, last)
	if last > 3*time.Second {
		t.Errorf("滚动到底耗时 %v，超过 3s", last)
	}
}

func tail(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// 阶段 2 验收：字体切换写入 Markdown 主题。
func TestFontChange(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	tt := ui.NewTester(a.view, 900, 700)
	if a.md.Theme == nil {
		t.Fatal("Markdown 主题未初始化")
	}
	before := a.md.Theme.Font

	a.font = "monospace"
	a.codeFont = "monospace"
	tt.Frame()
	if a.md.Theme.Font != "monospace" || a.md.Theme.CodeFont != "monospace" {
		t.Errorf("字体未生效：正文 %q 代码 %q", a.md.Theme.Font, a.md.Theme.CodeFont)
	}
	if before == "monospace" {
		t.Log("窗口默认字体本来就是 monospace")
	}
}

// 阶段 2 验收：代码块自带复制按钮，复制出纯代码。
func TestCodeBlockCopy(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	tt := ui.NewTester(a.view, 900, 700)
	candidates := []string{"Copy code", "Copy", "复制代码", "复制"}
	var clicked string
	for _, c := range candidates {
		if err := tt.Click(c); err == nil {
			clicked = c
			break
		}
	}
	if clicked == "" {
		t.Fatalf("没有找到复制按钮，文本 %q", tt.Texts())
	}
	got := tt.Clipboard()
	if !strings.Contains(got, `fmt.Println("你好")`) {
		t.Errorf("剪贴板内容不正确：%q", got)
	}
	if strings.Contains(got, "func main() {") == false {
		t.Errorf("剪贴板缺少完整代码：%q", got)
	}
	t.Logf("点击 %q，剪贴板 %q", clicked, got)
}

// 阶段 3 验收：打印视图 HTML 结构正确。
func TestRenderPrintHTML(t *testing.T) {
	page, err := renderPrintHTML("示例", filepath.Join(t.TempDir(), "sample.md"), sampleMarkdown+"\n<script>alert(1)</script>\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<title>示例</title>",
		"@page { size: A4",
		"<h1",
		"标题一",
		"<table>",
		"<pre><code",
		"正文段落",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("HTML 缺少 %q", want)
		}
	}
	if strings.Contains(page, "<script>") {
		t.Error("打印视图不应包含原始 HTML 脚本")
	}
}

func TestRenderPrintHTMLRelativeResources(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "note.md")
	page, err := renderPrintHTML("相对资源", source, "![图片](images/pic.png)\n\n[文档](other.md)\n")
	if err != nil {
		t.Fatal(err)
	}
	base := string(directoryBaseURL(dir, runtime.GOOS == "windows"))
	for _, want := range []string{
		`<base href="` + base + `">`,
		`<img src="images/pic.png"`,
		`<a href="other.md">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("HTML 缺少 %q", want)
		}
	}
}

func TestDirectoryBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		dir     string
		windows bool
		want    string
	}{
		{"Unix 路径编码", "/tmp/my docs", false, "file:///tmp/my%20docs/"},
		{"Windows 盘符", `C:\my docs`, true, "file:///C:/my%20docs/"},
		{"Windows UNC", `\\server\share\my docs`, true, "file://server/share/my%20docs/"},
	}
	for _, c := range cases {
		if got := string(directoryBaseURL(c.dir, c.windows)); got != c.want {
			t.Errorf("%s：得到 %q，期望 %q", c.name, got, c.want)
		}
	}
}

// 临时文件命名与读取。
func TestWriteTempHTML(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "我的 笔记.md")
	if err := os.WriteFile(src, []byte("# hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := writeTempHTML(src, "<html></html>")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(out)
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "<html></html>" {
		t.Errorf("写入内容 %q", b)
	}
	if !strings.Contains(filepath.Base(out), ".html") {
		t.Errorf("临时文件名 %q", out)
	}
	t.Logf("临时文件 %s", out)
}

func TestWriteTempHTMLLongName(t *testing.T) {
	src := filepath.Join(t.TempDir(), strings.Repeat("文", 200)+".md")
	out, err := writeTempHTML(src, "<html></html>")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(out)
	if got := len([]rune(sanitizeName(strings.Repeat("文", 200)))); got != tempNameLimit {
		t.Errorf("临时文件名前缀 %d 个 rune，期望 %d", got, tempNameLimit)
	}
	if len(filepath.Base(out)) > 255 {
		t.Errorf("临时文件名过长：%q", filepath.Base(out))
	}
}

func TestOpenInBrowserMissingCommand(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("仅验证 Linux 默认浏览器命令")
	}
	t.Setenv("PATH", t.TempDir())
	if err := openInBrowser(filepath.Join(t.TempDir(), "print.html")); err == nil {
		t.Fatal("浏览器命令缺失时应返回错误")
	}
}

// 读取真实文件，覆盖不存在路径。
func TestReadMarkdown(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	if err := os.WriteFile(p, []byte("# 内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := readMarkdown(p)
	if err != nil || src != "# 内容" {
		t.Fatalf("读到 %q，err %v", src, err)
	}
	if _, err := readMarkdown(filepath.Join(dir, "missing.md")); err == nil {
		t.Error("读取不存在的文件应返回错误")
	}
}

// 渲染快照，供人工核对排版：SHOT=/tmp/x.png go test -run TestScreenshot
func TestScreenshot(t *testing.T) {
	out := os.Getenv("SHOT")
	if out == "" {
		t.Skip("未设置 SHOT")
	}
	src := sampleMarkdown + "\n> 引用一段话，含中文标点：你好，世界。\n\n- [x] 已完成\n- [ ] 未完成\n"
	a := newTestApp(src)
	if v := os.Getenv("SHOT_WIDTH"); v != "" {
		fmt.Sscanf(v, "%d", &a.widthMode)
	}
	if v := os.Getenv("SHOT_ZOOM"); v != "" {
		fmt.Sscanf(v, "%d", &a.zoom)
	}
	if v := os.Getenv("SHOT_FONT"); v != "" {
		fmt.Sscanf(v, "%f", &a.fontSize)
	}
	if v := os.Getenv("SHOT_FACE"); v != "" {
		a.font = v
		a.codeFont = v
	}
	w, h := 900, 700
	if v := os.Getenv("SHOT_W"); v != "" {
		fmt.Sscanf(v, "%dx%d", &w, &h)
	}
	tt := ui.NewTester(a.view, w, h)
	if os.Getenv("SHOT_BAR") == "hide" {
		tt.Move(float32(w)/2, float32(h)/2)
		tt.Advance(barInitialDelay + time.Second)
	}
	if os.Getenv("SHOT_FONTS") != "" {
		if err := tt.Click("字体"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	if err := os.WriteFile(out, encodePNG(tt.Image()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("写入 %s", out)
}

func encodePNG(img *image.RGBA) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// 字体列表来自系统，至少包含一个真实字体族。
func TestFontListLoaded(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	tt := ui.NewTester(a.view, 900, 700)
	if err := tt.Click("字体"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.fonts) < 2 {
		t.Errorf("字体列表只有 %d 项：%v", len(a.fonts), a.fonts)
	}
	found := false
	for _, f := range a.fonts {
		if strings.Contains(f, "Noto") || strings.Contains(f, "Sans") || strings.Contains(f, "雅黑") {
			found = true
			break
		}
	}
	if !found {
		t.Logf("列表中没有常见中文字体名，前几项 %v", a.fonts[:min(len(a.fonts), 8)])
	}
	t.Logf("字体族 %d 个，例如 %v", len(a.fonts)-1, a.fonts[1:min(len(a.fonts), 6)])
}

func visibleSystemFont(texts []string) string {
	for _, family := range ui.FontFamilies() {
		if contains(texts, family.Name) {
			return family.Name
		}
	}
	return ""
}

// 点开字体下拉后选项出现在视图里。
func TestFontSelectOpens(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	tt := ui.NewTester(a.view, 900, 700)
	if err := tt.Click("字体"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("默认"); err != nil {
		t.Fatalf("点击字体选择器失败：%v", err)
	}
	tt.Frame()
	texts := tt.Texts()
	if visibleSystemFont(texts) == "" {
		t.Fatalf("下拉列表没有显示系统字体，文本共 %d 条：%v", len(texts), texts)
	}
}

// 把打印视图写成文件，供浏览器打印实测：PRINT_HTML=/tmp/x.html go test -run TestPrintHTMLFile
func TestPrintHTMLFile(t *testing.T) {
	out := os.Getenv("PRINT_HTML")
	if out == "" {
		t.Skip("未设置 PRINT_HTML")
	}
	src := sampleMarkdown + "\n" + strings.Repeat("\n段落内容用于分页测试。中文排版应保持完整，不应在行中被截断。\n", 40)
	page, err := renderPrintHTML("打印测试", filepath.Join(filepath.Dir(out), "source.md"), src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("写入 %s（%d 字节）", out, len(page))
}

// 宽度档位：60/80/100 为可用宽度的百分比。
func TestContentWidth(t *testing.T) {
	cases := []struct {
		mode    int
		percent float32
	}{
		{0, 60},
		{1, 80},
		{2, 100},
	}
	for _, c := range cases {
		a := &app{widthMode: c.mode}
		if got := a.contentWidth(); got != c.percent {
			t.Errorf("模式 %d：得到 %v，期望 %v", c.mode, got, c.percent)
		}
	}
}

// 切换宽度档位后视图重新布局不崩溃，且分段控件可达。
func TestWidthSwitching(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	a.widthMode = defaultWidthMode
	tt := ui.NewTester(a.view, 900, 700)
	for _, label := range []string{"60%", "100%", "80%"} {
		if err := tt.Click(label); err != nil {
			t.Fatalf("点击 %q 失败：%v", label, err)
		}
		tt.Frame()
		if !tt.HasText("标题一") {
			t.Errorf("切到 %q 后正文消失", label)
		}
	}
}

// 字体设置写入后能读回，路径的目录不存在时自动创建。
func TestPrefsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "settings.json")
	want := prefs{Font: "DejaVu Sans", CodeFont: "JetBrains Mono", FontSize: 16}
	if err := savePrefs(path, want); err != nil {
		t.Fatal(err)
	}
	if got := loadPrefs(path); got != want {
		t.Errorf("读回 %+v，期望 %+v", got, want)
	}
	// 再写一次覆盖旧值。
	want = prefs{Font: "Noto Sans CJK SC", FontSize: 12}
	if err := savePrefs(path, want); err != nil {
		t.Fatal(err)
	}
	if got := loadPrefs(path); got != want {
		t.Errorf("覆盖后读回 %+v，期望 %+v", got, want)
	}
}

// 文件缺失、内容损坏都退回默认，不让启动失败。
func TestPrefsFallbacks(t *testing.T) {
	dir := t.TempDir()
	if got := loadPrefs(filepath.Join(dir, "missing.json")); got != (prefs{}) {
		t.Errorf("缺失文件返回 %+v", got)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadPrefs(bad); got != (prefs{}) {
		t.Errorf("损坏文件返回 %+v", got)
	}
	if got := loadPrefs(""); got != (prefs{}) {
		t.Errorf("空路径返回 %+v", got)
	}
	if err := savePrefs("", prefs{}); err == nil {
		t.Error("空路径保存应返回错误")
	}
}

func TestPrefsRejectsInvalidFontSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, size := range []float64{-1, fontSizeMin - 1, fontSizeMax + 1, 1000} {
		if err := savePrefs(path, prefs{Font: "Test Font", FontSize: size}); err != nil {
			t.Fatal(err)
		}
		got := loadPrefs(path)
		if got.FontSize != 0 {
			t.Errorf("字号 %v 未回退：%+v", size, got)
		}
		if got.Font != "Test Font" {
			t.Errorf("非法字号不应丢弃字体：%+v", got)
		}
	}
	for _, size := range []float64{fontSizeMin, fontSizeMax} {
		if err := savePrefs(path, prefs{FontSize: size}); err != nil {
			t.Fatal(err)
		}
		if got := loadPrefs(path).FontSize; got != size {
			t.Errorf("边界字号 %v 被改成 %v", size, got)
		}
	}
}

// 保存的字体在系统里不存在时，下拉回退默认并清空字体。
func TestMissingFontFallsBack(t *testing.T) {
	a := &app{fontSel: "不存在的字体", codeSel: "另一个不存在的字体", font: "不存在的字体", codeFont: "另一个不存在的字体"}
	a.widthMode = defaultWidthMode
	tt := ui.NewTester(a.view, 900, 700)
	if err := tt.Click("字体"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.fontSel != defaultFontLabel || a.font != "" {
		t.Errorf("正文字体未回退：选择 %q 字体 %q", a.fontSel, a.font)
	}
	if a.codeSel != defaultFontLabel || a.codeFont != "" {
		t.Errorf("代码字体未回退：选择 %q 字体 %q", a.codeSel, a.codeFont)
	}
}

// 保存的字体在系统里存在时保留。
func TestSavedFontKept(t *testing.T) {
	families := ui.FontFamilies()
	if len(families) == 0 {
		t.Skip("系统没有可用字体族")
	}
	target := families[0].Name
	a := &app{fontSel: target, codeSel: target, font: target, codeFont: target}
	a.widthMode = defaultWidthMode
	tt := ui.NewTester(a.view, 900, 700)
	if err := tt.Click("字体"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.font != target || a.codeFont != target {
		t.Errorf("保存的字体被改动：%q / %q", a.font, a.codeFont)
	}
	if a.md.Theme.Font != target {
		t.Errorf("Markdown 主题字体 %q", a.md.Theme.Font)
	}
}

// 端到端：在字体面板里选一个字体，写入设置文件，下次启动读回。
func TestFontChoicePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	a := newTestApp(sampleMarkdown)
	a.prefsPath = path
	a.fontSel, a.codeSel = defaultFontLabel, defaultFontLabel
	tt := ui.NewTester(a.view, 900, 700)

	if err := tt.Click("字体"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("默认"); err != nil {
		t.Fatalf("打开正文字体下拉失败：%v", err)
	}
	tt.Frame()

	target := visibleSystemFont(tt.Texts())
	if target == "" {
		t.Skip("下拉列表中没有可见的系统字体")
	}
	if err := tt.Click(target); err != nil {
		t.Fatalf("点击字体 %q 失败：%v", target, err)
	}
	tt.Frame()
	if a.font != target {
		t.Fatalf("选择后字体 %q，期望 %q", a.font, target)
	}
	saved := loadPrefs(path)
	if saved.Font != target {
		t.Fatalf("设置文件里是 %+v，期望 Font=%q", saved, target)
	}

	// 模拟下一次启动：新 app 读取同一个文件。
	b := newTestApp(sampleMarkdown)
	b.fontSel, b.codeSel = defaultFontLabel, defaultFontLabel
	b.prefsPath = path
	p := loadPrefs(b.prefsPath)
	b.font, b.codeFont = p.Font, p.CodeFont
	b.fontSel, b.codeSel = labelOf(p.Font), labelOf(p.CodeFont)
	if b.font != target {
		t.Errorf("重启后字体 %q，期望 %q", b.font, target)
	}
	tt2 := ui.NewTester(b.view, 900, 700)
	if b.md.Theme.Font != target {
		t.Errorf("重启后 Markdown 主题字体 %q", b.md.Theme.Font)
	}
	if !tt2.HasText("标题一") {
		t.Error("重启后正文未渲染")
	}
}

// Ctrl+滚轮按格缩放，无 Ctrl 的滚轮与触摸板滑动留给滚动容器。
func TestZoomInput(t *testing.T) {
	cases := []struct {
		name string
		ev   ui.InputEvent
		cur  int
		want int
		ok   bool
	}{
		{"Ctrl 上滚放大", ui.InputEvent{Kind: ui.InputScroll, DY: -1, Mods: ui.Ctrl}, 100, 110, true},
		{"Ctrl 下滚缩小", ui.InputEvent{Kind: ui.InputScroll, DY: 1, Mods: ui.Ctrl}, 100, 90, true},
		{"无修饰键不缩放", ui.InputEvent{Kind: ui.InputScroll, DY: -1}, 100, 100, false},
		{"触摸板滑动不缩放", ui.InputEvent{Kind: ui.InputScroll, DY: -1, Mods: ui.Ctrl, Precise: true}, 100, 100, false},
		{"捏合放大两倍", ui.InputEvent{Kind: ui.InputZoom, Scale: 2}, 100, 200, true},
		{"捏合缩小一半", ui.InputEvent{Kind: ui.InputZoom, Scale: 0.5}, 100, 50, true},
		{"上限钳制", ui.InputEvent{Kind: ui.InputScroll, DY: -1, Mods: ui.Ctrl}, zoomMax, zoomMax, true},
		{"下限钳制", ui.InputEvent{Kind: ui.InputScroll, DY: 1, Mods: ui.Ctrl}, zoomMin, zoomMin, true},
		{"其他输入不处理", ui.InputEvent{Kind: ui.InputText}, 100, 100, false},
	}
	for _, c := range cases {
		got, ok := zoomInput(c.ev, c.cur)
		if got != c.want || ok != c.ok {
			t.Errorf("%s：得到 (%d, %v)，期望 (%d, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

// 放大/缩小按钮改变缩放，并按比例改变正文字号。
func TestZoomButtons(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	tt := ui.NewTester(a.view, 900, 700)
	base := a.md.Theme.FontSize
	if a.zoom != defaultZoom {
		t.Fatalf("初始缩放 %d", a.zoom)
	}

	if err := tt.Click("放大"); err != nil {
		t.Fatal(err)
	}
	if a.zoom != 110 {
		t.Errorf("放大后缩放 %d", a.zoom)
	}
	if got := a.md.Theme.FontSize; got <= base {
		t.Errorf("字号没有变大：%v → %v", base, got)
	}
	if !tt.HasText("110%") {
		t.Errorf("工具栏没有显示当前比例，文本 %q", tt.Texts())
	}

	if err := tt.Click("缩小"); err != nil {
		t.Fatal(err)
	}
	if a.zoom != 100 {
		t.Errorf("缩小后缩放 %d", a.zoom)
	}

	// 点比例回到默认。
	tt.Click("放大")
	tt.Click("放大")
	if err := tt.Click("缩放比例，点击重置"); err != nil {
		t.Fatal(err)
	}
	if a.zoom != defaultZoom {
		t.Errorf("重置后缩放 %d", a.zoom)
	}
}

// 缩放到达上下限后不再变化。
func TestZoomLimits(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	a.zoom = zoomMax
	tt := ui.NewTester(a.view, 900, 700)
	tt.Click("放大")
	if a.zoom != zoomMax {
		t.Errorf("超过上限：缩放 %d", a.zoom)
	}
	a.zoom = zoomMin
	tt.Frame()
	tt.Click("缩小")
	if a.zoom != zoomMin {
		t.Errorf("低于下限：缩放 %d", a.zoom)
	}
}

// 触摸板捏合走同一条缩放路径。
func TestZoomPinch(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	tt := ui.NewTester(a.view, 900, 700)
	tt.Pinch(450, 350, 2)
	if a.zoom != 200 {
		t.Errorf("捏合后缩放 %d，期望 200", a.zoom)
	}
	tt.Frame()
	if !tt.HasText("200%") {
		t.Errorf("工具栏比例未更新，文本 %q", tt.Texts())
	}
}

// 标题字重比框架默认更粗。
func TestHeadingWeights(t *testing.T) {
	src := "# 一级标题 H1\n\n## 二级标题 H2\n\n### 三级标题 H3\n\n#### 四级标题 H4\n\n正文用来对比粗细程度。\n"
	a := newTestApp(src)
	tt := ui.NewTester(a.view, 760, 520)
	want := [6]int{800, 750, 700, 700, 700, 700}
	for i, w := range want {
		if a.md.Theme.Headings[i].Weight != w {
			t.Errorf("h%d 字重 %d，期望 %d", i+1, a.md.Theme.Headings[i].Weight, w)
		}
	}
	if out := os.Getenv("SHOT"); out != "" {
		if err := os.WriteFile(out, encodePNG(tt.Image()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// 字号改变会写入设置文件，重启后字号与主题一致。
func TestFontSizePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	a := newTestApp(sampleMarkdown)
	a.prefsPath = path
	tt := ui.NewTester(a.view, 900, 700)

	if a.md.Theme.FontSize != float32(defaultFontSizeForOS()) {
		t.Fatalf("初始字号 %v，期望 %v", a.md.Theme.FontSize, defaultFontSizeForOS())
	}
	if err := tt.Click("字体"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// 点输入区（避开右侧步进箭头），全选后键入。
	box, ok := tt.Find("正文字号")
	if !ok {
		t.Fatal("找不到字号输入框")
	}
	tt.ClickAt(box.X+18, box.Y+box.H/2)
	tt.Command("selectAll")
	tt.Type("16")
	tt.Frame()
	if a.fontSize != 16 {
		t.Fatalf("字号 %v，期望 16", a.fontSize)
	}
	if a.md.Theme.FontSize != float32(a.fontSize) {
		t.Errorf("主题字号 %v 与设置 %v 不一致", a.md.Theme.FontSize, a.fontSize)
	}
	if got := loadPrefs(path).FontSize; got != a.fontSize {
		t.Errorf("设置文件字号 %v，期望 %v", got, a.fontSize)
	}

	// 模拟重启。
	b := newTestApp(sampleMarkdown)
	b.prefsPath = path
	p := loadPrefs(b.prefsPath)
	b.fontSize = p.FontSize
	tt2 := ui.NewTester(b.view, 900, 700)
	if b.md.Theme.FontSize != 16 {
		t.Errorf("重启后主题字号 %v", b.md.Theme.FontSize)
	}
	if !tt2.HasText("标题一") {
		t.Error("重启后正文未渲染")
	}
}

// 字号与缩放叠加：字号 16、缩放 150% 时主题字号为 24。
func TestFontSizeWithZoom(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	a.fontSize = 16
	a.zoom = 150
	tt := ui.NewTester(a.view, 900, 700)
	if got, want := a.md.Theme.FontSize, float32(24); got != want {
		t.Errorf("主题字号 %v，期望 %v", got, want)
	}
	tt.Frame()
	if !tt.HasText("标题一") {
		t.Error("正文未渲染")
	}
}

// DPI 换算：Xft.dpi 与已生效的整数缩放共同决定界面缩放。
func TestScaleFromDPI(t *testing.T) {
	cases := []struct {
		name    string
		dpi     float64
		monitor float64
		want    float32
	}{
		{"无 DPI 不缩放", 0, 1, 1},
		{"96 不缩放", 96, 1, 1},
		{"144 放大 1.5 倍", 144, 1, 1.5},
		{"192 放大 2 倍", 192, 1, 2},
		{"缩放系数已生效则不重复放大", 192, 2, 1},
		{"缩放系数 2 且 DPI 144 则减半", 144, 2, 0.75},
		{"低于下限钳制", 72, 1, 0.75},
		{"高于上限钳制", 480, 1, 3},
	}
	for _, c := range cases {
		if got := scaleFromDPI(c.dpi, c.monitor); got != c.want {
			t.Errorf("%s：得到 %v，期望 %v", c.name, got, c.want)
		}
	}
}

// 正文行高比框架默认更舒展。
func TestBodyLineHeight(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	tt := ui.NewTester(a.view, 900, 700)
	if got := a.md.Theme.LineHeight; got != bodyLineHeight {
		t.Errorf("行高 %v，期望 %v", got, bodyLineHeight)
	}
	tt.Frame()
	if !tt.HasText("标题一") {
		t.Error("正文未渲染")
	}
}

// 在真实 X 会话里读取桌面的 Xft.dpi。无 DISPLAY 或 xrdb 不可用时跳过。
func TestXftDPIReadsDesktop(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("没有 DISPLAY")
	}
	dpi := xftDPI()
	if dpi == 0 {
		t.Skip("xrdb 读不到 Xft.dpi")
	}
	t.Logf("Xft.dpi = %v，界面缩放 = %v", dpi, uiScale(1))
	if dpi <= 0 {
		t.Errorf("DPI 非正：%v", dpi)
	}
}

// 工具栏启动时显示，到点后隐藏；显示状态直接决定是否渲染。
// 指针唤出依赖 HandleInput 的指针事件，Tester 不产生这类事件，由真实窗口验证。
func TestBarAutoHide(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	tt := ui.NewTester(a.view, 900, 700)
	if !tt.HasText("打开…") {
		t.Fatal("启动时工具栏应可见")
	}

	tt.Advance(barInitialDelay + 500*time.Millisecond)
	tt.Frame()
	if a.barVisible {
		t.Error("启动一段时间后工具栏应隐藏")
	}
	if tt.HasText("打开…") {
		t.Error("隐藏后不应渲染工具栏")
	}

	a.barVisible = true
	tt.Frame()
	if !tt.HasText("打开…") {
		t.Error("显示状态下应渲染工具栏")
	}
}

func TestBarHideDelayAndHold(t *testing.T) {
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	a := &app{barVisible: true, pointerInside: true, pointerY: 200}
	a.updateBarVisibility(now)
	if !a.barHideAt.Equal(now.Add(barHideDelay)) {
		t.Fatalf("隐藏时间 %v，期望 %v", a.barHideAt, now.Add(barHideDelay))
	}
	a.updateBarVisibility(now.Add(barHideDelay - time.Nanosecond))
	if !a.barVisible {
		t.Fatal("延迟结束前工具栏不应隐藏")
	}
	a.updateBarVisibility(now.Add(barHideDelay))
	if a.barVisible {
		t.Fatal("延迟结束后工具栏应隐藏")
	}

	a.barVisible = true
	a.barHovered = true
	a.updateBarVisibility(now)
	if !a.barVisible || !a.barHideAt.IsZero() {
		t.Fatal("指针悬停工具栏时应保持显示且不安排隐藏")
	}
	a.barHovered = false
	a.pointerY = barTrigger
	a.updateBarVisibility(now)
	if !a.barVisible || !a.barHideAt.IsZero() {
		t.Fatal("指针在顶部感应区时应保持显示")
	}
	a.pointerInside = false
	a.showFonts = true
	a.updateBarVisibility(now)
	if !a.barVisible {
		t.Fatal("字体面板打开时工具栏不应隐藏")
	}
}

func TestBarWrappedToolbarStaysHovered(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	a.path = "/tmp/a-very-long-markdown-file-name-for-toolbar-wrapping.md"
	tt := ui.NewTester(a.view, 420, 700)
	box, ok := tt.Find("导出 PDF")
	if !ok {
		t.Fatal("找不到导出按钮")
	}
	x, y := box.X+box.W/2, box.Y+box.H/2
	if y <= 64 {
		t.Fatalf("窄窗口未复现换行工具栏，按钮中心 Y=%v", y)
	}
	tt.Move(x, y)
	tt.Frame()
	tt.Advance(barInitialDelay + 2*time.Second)
	tt.Frame()
	if !a.barVisible || !tt.HasText("导出 PDF") {
		t.Fatal("指针悬停换行后的工具栏时被隐藏")
	}
}

func TestErrorAndNoticeShowBar(t *testing.T) {
	a := &app{err: "旧错误", notice: "旧提示"}
	a.showError("新错误")
	if !a.barVisible || a.err != "新错误" || a.notice != "" {
		t.Fatalf("错误提示状态不正确：%+v", a)
	}
	a.showNotice("新提示")
	if !a.barVisible || a.err != "" || a.notice != "新提示" {
		t.Fatalf("普通提示状态不正确：%+v", a)
	}
}

// 字体面板打开时工具栏保持可见。
func TestBarStaysOpenWithFonts(t *testing.T) {
	a := newTestApp(sampleMarkdown)
	tt := ui.NewTester(a.view, 900, 700)
	tt.Move(450, 400)
	tt.Frame()
	if err := tt.Click("字体"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(barInitialDelay + time.Second)
	if !tt.HasText("正文字体") {
		t.Error("字体面板打开时不应随工具栏隐藏")
	}
}

// 改名后把旧配置目录里的设置迁移过来，已有设置时不覆盖。
func TestMigrateLegacySettings(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, legacyAppName)
	newDir := filepath.Join(root, appName)
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(oldDir, "settings.json")
	if err := os.WriteFile(oldPath, []byte(`{"fontSize":16}`), 0o644); err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(newDir, "settings.json")

	migrateLegacySettings(newDir, newPath)
	if got := loadPrefs(newPath).FontSize; got != 16 {
		t.Errorf("迁移后字号 %v，期望 16", got)
	}

	// 新目录已有设置时不覆盖。
	if err := os.WriteFile(newPath, []byte(`{"fontSize":12}`), 0o644); err != nil {
		t.Fatal(err)
	}
	migrateLegacySettings(newDir, newPath)
	if got := loadPrefs(newPath).FontSize; got != 12 {
		t.Errorf("不应覆盖已有设置，得到 %v", got)
	}
}
