package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// 应用名。不带空格：它会成为配置目录名、deb 的包名与桌面条目名。
const appName = "MarkdownViewer"

// 旧版应用名，用于把配置目录里的设置迁移过来。
const legacyAppName = "Markdown Viewer"

// 工具栏自动隐藏：启动后显示这么久，指针离开后再停这么久。
const (
	barInitialDelay = 3 * time.Second
	barHideDelay    = 1500 * time.Millisecond
)

// 工具栏的位置阈值，单位 DIP：指针进到 barTrigger 内显示。
const barTrigger = 8

// 轮询指针的间隔。MyGo 的指针事件在部分窗口几何下会丢失，轮询是兜底。
const (
	barPollShown  = 150 * time.Millisecond
	barPollHidden = 250 * time.Millisecond
)

// 空字符串表示跟随窗口主题的字体。
const defaultFontLabel = "默认"

// 内容宽度的档位：可用宽度的百分比。
var widthLabels = []string{"60%", "80%", "100%"}

// 默认 80%：窗口变宽时正文随之变宽，又不至于行长过长。
const defaultWidthMode = 1

// 字号缩放：百分比，步进 10%，限制在 50%–250%。
const (
	defaultZoom = 100
	zoomMin     = 50
	zoomMax     = 250
	zoomStep    = 10
)

// 正文字号，单位 DIP：Linux 默认 14、Windows 默认 16，可调范围 10–28。
const (
	fontSizeMin  = 10
	fontSizeMax  = 28
	fontSizeStep = 1
)

// 正文行高，1.6 比框架默认的 1.5 舒展，接近桌面编辑器的观感。
const bodyLineHeight = 1.6

// defaultFontSizeForOS 返回各平台的默认正文字号。
func defaultFontSizeForOS() float64 {
	if runtime.GOOS == "windows" {
		return 16
	}
	return 14
}

// defaultFontForOS 返回各平台的默认正文字体。空字符串表示跟随窗口主题。
// Windows 上用微软雅黑：它是为低 DPI 中文优化过的，笔画均匀、边缘柔和。
func defaultFontForOS() string {
	if runtime.GOOS == "windows" {
		return "Microsoft YaHei"
	}
	return ""
}

// app 是窗口显示的状态，view 每帧从它构建界面。
type app struct {
	win       *mygo.Window
	path      string
	src       string
	err       string
	notice    string
	exporting bool

	md       markdown.Options
	themeKey string

	font          string
	codeFont      string
	fontSize      float64
	fonts         []string
	fontSel       string
	codeSel       string
	showFonts     bool
	widthMode     int
	zoom          int
	prefsPath     string
	uiZoom        float32
	barVisible    bool
	barInit       bool
	barStartHide  time.Time
	barHideAt     time.Time
	barHovered    bool
	pointerY      float32
	pointerInside bool
	screenScale   float32
	scroll        ui.ScrollState
}

func main() {
	a := &app{showFonts: false, fontSel: defaultFontLabel, codeSel: defaultFontLabel, widthMode: defaultWidthMode, zoom: defaultZoom, fontSize: defaultFontSizeForOS(), uiZoom: 1}
	a.loadPrefs()
	mygo.App.WhenReady(func() {
		// Screen 需要在应用就绪后访问，放在这里算界面的 DPI 缩放。
		disp := mygo.Screen.PrimaryDisplay()
		a.uiZoom = uiScale(disp.ScaleFactor)
		a.screenScale = float32(disp.ScaleFactor) * a.uiZoom
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     appName,
			Width:     980,
			Height:    760,
			MinWidth:  420,
			MinHeight: 320,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		// 命令行参数里给出文件时直接打开。
		if len(os.Args) > 1 {
			a.openPath(os.Args[1])
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// loadPrefs 在 Run 之前读取保存的字体，App.Path 此时已可用。
func (a *app) loadPrefs() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Printf("读取设置目录失败，字体不会被保存：%v", err)
		return
	}
	a.prefsPath = filepath.Join(dir, "settings.json")
	migrateLegacySettings(dir, a.prefsPath)
	p := loadPrefs(a.prefsPath)
	a.font, a.codeFont = p.Font, p.CodeFont
	if a.font == "" {
		a.font = defaultFontForOS()
	}
	a.fontSel, a.codeSel = labelOf(a.font), labelOf(a.codeFont)
	a.fontSize = defaultFontSizeForOS()
	if p.FontSize > 0 {
		a.fontSize = p.FontSize
	}
}

func (a *app) showError(message string) {
	a.notice = ""
	a.err = message
	a.barVisible = true
	a.barHideAt = time.Time{}
}

func (a *app) showNotice(message string) {
	a.err = ""
	a.notice = message
	a.barVisible = true
	a.barHideAt = time.Time{}
}

// savePrefs 写入当前字体，失败时由调用方提示。
func (a *app) savePrefs() error {
	return savePrefs(a.prefsPath, prefs{Font: a.font, CodeFont: a.codeFont, FontSize: a.fontSize})
}

// labelOf 把保存的字体名转成下拉选项的标签。
func labelOf(family string) string {
	if family == "" {
		return defaultFontLabel
	}
	return family
}

func (a *app) view(c *ui.Context) {
	// 桌面把 DPI 设得比 96 高时按比例放大整个界面，与浏览器缩放页面一样按更高像素密度绘制。
	if a.uiZoom != 1 {
		c.SetZoom(a.uiZoom)
	}
	t := c.Theme()
	a.markdownTheme(c)
	a.updateBar(c)
	ui.Column(c).Fill().Children(func() {
		ui.Box(c).Fill().HandleInput(func(ev ui.InputEvent) bool {
			a.trackPointer(c, ev)
			next, ok := zoomInput(ev, a.zoom)
			if !ok {
				return false
			}
			a.zoom = next
			c.Invalidate()
			return true
		}).Children(func() {
			ui.Scroll(c).TrackScroll(&a.scroll).Grow(1).MinHeight(0).Padding(0).Children(func() {
				if a.src == "" {
					ui.Column(c).Fill().Center().Gap(10).Padding(40).Children(func() {
						ui.Text(c, "用“打开”选择一个 Markdown 文件").TextColor(t.TextMuted)
						ui.Text(c, "支持 CommonMark、GFM 表格、任务列表、脚注与代码高亮").FontSize(13).TextColor(t.TextMuted)
					})
					return
				}
				col := ui.Column(c).AlignSelf(ui.Center).Padding(20, 24, 48, 24).
					WidthPercent(a.contentWidth())
				col.Children(func() {
					markdown.View(c, a.src, &a.md)
				})
			})
		})
	})
	a.barOverlay(c, t)
}

// updateBar 推进启动显示、延迟隐藏和指针轮询；周期帧用于兜住离开窗口等没有事件的状态。
func (a *app) updateBar(c *ui.Context) {
	if !a.barInit {
		a.barInit = true
		a.barVisible = true
		a.barStartHide = c.Now().Add(barInitialDelay)
	}
	now := c.Now()
	if !a.barStartHide.IsZero() && !now.Before(a.barStartHide) {
		a.barHideAt = a.barStartHide
		a.barStartHide = time.Time{}
	}
	a.pollPointer(c)
	a.updateBarVisibility(now)
	if a.barVisible {
		c.After(barPollShown)
	} else {
		c.After(barPollHidden)
	}
}

func (a *app) updateBarVisibility(now time.Time) {
	if a.showFonts || a.barHovered || (a.pointerInside && a.pointerY <= barTrigger) {
		a.barVisible = true
		a.barHideAt = time.Time{}
		return
	}
	if !a.barVisible {
		a.barHideAt = time.Time{}
		return
	}
	if a.barHideAt.IsZero() {
		a.barHideAt = now.Add(barHideDelay)
		return
	}
	if !now.Before(a.barHideAt) {
		a.barVisible = false
		a.barHideAt = time.Time{}
	}
}

// pollPointer 兜底：按屏幕坐标判断指针是否贴在内容顶部，并记录是否离开了窗口。
// 指针事件在窗口装饰边缘等位置会丢失，光靠 trackPointer 会漏掉。
func (a *app) pollPointer(c *ui.Context) {
	if a.win == nil {
		return
	}
	cur := mygo.Screen.CursorScreenPoint()
	cb := a.win.ContentBounds()
	a.pointerInside = cur.X >= cb.X && cur.X <= cb.X+cb.Width &&
		cur.Y >= cb.Y && cur.Y <= cb.Y+cb.Height
	if !a.pointerInside {
		return
	}
	a.pointerY = float32(cur.Y-cb.Y) / a.screenScale
	if a.pointerY <= barTrigger {
		a.barVisible = true
		a.barHideAt = time.Time{}
	}
}

// trackPointer 在指针进入顶部感应区时显示工具栏；显示后的隐藏交给延迟状态机。
func (a *app) trackPointer(c *ui.Context, ev ui.InputEvent) {
	if ev.Kind != ui.InputPointerMove {
		return
	}
	a.pointerInside = true
	a.pointerY = ev.Y
	if ev.Y <= barTrigger && !a.barVisible {
		a.barVisible = true
		a.barHideAt = time.Time{}
		c.Invalidate()
	}
}

// barOverlay 把工具栏浮在正文之上：顶部留一条窄的感应区，指针进入时显示。
func (a *app) barOverlay(c *ui.Context, t *ui.Theme) {
	a.barHovered = false
	ui.Overlay(c, func() {
		if !a.barVisible {
			return
		}
		bar := ui.Column(c).Absolute().Top(0).Left(0).Right(0).
			Background(t.Background).Border(1, t.Border)
		bar.Children(func() {
			a.toolbar(c, t)
			if a.notice != "" {
				ui.Text(c, a.notice).TextColor(t.TextMuted).Padding(0, 20, 8, 20)
			}
			if a.err != "" {
				ui.Text(c, a.err).TextColor(t.Danger).Padding(0, 20, 8, 20)
			}
			if a.showFonts {
				a.fontPanel(c, t)
			}
		})
		if bar.Hovered() {
			a.barHovered = true
			a.barVisible = true
			a.barHideAt = time.Time{}
		}
	})
}

// contentWidth 返回内容列占可用宽度的百分比。
func (a *app) contentWidth() float32 {
	switch a.widthMode {
	case 0:
		return 60
	case 2:
		return 100
	default:
		return 80
	}
}

// toolbar 分组排列：组内间距一致，组与组之间留出更大的间隔。
func (a *app) toolbar(c *ui.Context, t *ui.Theme) {
	ui.Row(c).AlignItems(ui.Center).GapX(20).GapY(8).Wrap().Padding(10, 20).Shrink(0).Children(func() {
		// 文件操作组。
		ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
			if ui.PrimaryButton(c, "打开…").Clicked() {
				a.pickFile()
			}
			if a.path != "" {
				ui.Text(c, filepath.Base(a.path)).TextColor(t.TextMuted).
					SingleLine().TruncateMiddle().MaxWidth(220)
			}
		})
		// 视图宽度组。
		ui.Segmented(c, &a.widthMode, widthLabels...).Label("内容宽度")
		// 字号缩放组，与页面宽度设置隔开。
		ui.Row(c).Gap(6).AlignItems(ui.Center).MarginStart(24).Children(func() {
			minus := ui.Button(c, "−").Width(34).Label("缩小").Disabled(a.zoom <= zoomMin)
			if minus.Clicked() {
				a.zoomBy(-zoomStep, c)
			}
			reset := ui.Button(c, fmt.Sprintf("%d%%", a.zoom)).Width(78).Label("缩放比例，点击重置")
			if reset.Clicked() {
				a.zoomBy(defaultZoom-a.zoom, c)
			}
			plus := ui.Button(c, "+").Width(34).Label("放大").Disabled(a.zoom >= zoomMax)
			if plus.Clicked() {
				a.zoomBy(zoomStep, c)
			}
		})
		ui.Spacer(c)
		// 设置组。
		if ui.Button(c, "字体").Clicked() {
			a.showFonts = !a.showFonts
		}
		// 动作组。
		if ui.Button(c, "导出 PDF").Disabled(a.exporting).Clicked() {
			a.exportPDF(c)
		}
	})
}

// zoomBy 调整字号缩放，可正可负，结果限制在范围内。
func (a *app) zoomBy(delta int, c *ui.Context) {
	a.zoom = clampZoom(a.zoom + delta)
	c.Invalidate()
}

// clampZoom 把缩放百分比限制在允许范围内。
func clampZoom(z int) int {
	if z < zoomMin {
		return zoomMin
	}
	if z > zoomMax {
		return zoomMax
	}
	return z
}

// zoomInput 把输入解释成字号缩放：鼠标滚轮按住 Ctrl 时按格缩放，
// 触摸板捏合按比例缩放。其他输入返回 ok=false，留给滚动容器。
func zoomInput(ev ui.InputEvent, current int) (int, bool) {
	switch ev.Kind {
	case ui.InputZoom:
		if ev.Scale <= 0 {
			return current, false
		}
		return clampZoom(int(math.Round(float64(current) * float64(ev.Scale)))), true
	case ui.InputScroll:
		if ev.Precise || ev.Mods&ui.Ctrl == 0 {
			return current, false
		}
		switch {
		case ev.DY < 0:
			return clampZoom(current + zoomStep), true
		case ev.DY > 0:
			return clampZoom(current - zoomStep), true
		}
	}
	return current, false
}

func (a *app) fontPanel(c *ui.Context, t *ui.Theme) {
	if len(a.fonts) == 0 {
		a.loadFonts()
	}
	ui.Column(c).Gap(8).Padding(12, 20).Shrink(0).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "正文字体").FontSize(13).TextColor(t.TextMuted).Width(72)
			if ui.Select(c, &a.fontSel, a.fonts).Width(280).Changed() {
				a.font = familyOf(a.fontSel)
				a.persist(c)
			}
			ui.Text(c, "代码字体").FontSize(13).TextColor(t.TextMuted).Width(72)
			if ui.Select(c, &a.codeSel, a.fonts).Width(280).Changed() {
				a.codeFont = familyOf(a.codeSel)
				a.persist(c)
			}
		})
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "字号").FontSize(13).TextColor(t.TextMuted).Width(72)
			if ui.NumberInput(c, &a.fontSize, fontSizeMin, fontSizeMax, fontSizeStep).Width(96).Label("正文字号").Changed() {
				a.persist(c)
			}
			ui.Text(c, "DIP，10–28").FontSize(12).TextColor(t.TextMuted)
		})
		ui.Text(c, "字体与字号会保存，下次启动继续使用。").FontSize(12).TextColor(t.TextMuted)
	})
}

// loadFonts 收集可用字体族，在主线程上调用一次。
func (a *app) loadFonts() {
	fams := ui.FontFamilies()
	a.fonts = make([]string, 0, len(fams)+1)
	a.fonts = append(a.fonts, defaultFontLabel)
	for _, f := range fams {
		a.fonts = append(a.fonts, f.Name)
	}
	if !contains(a.fonts, a.fontSel) {
		a.fontSel, a.font = defaultFontLabel, ""
	}
	if !contains(a.fonts, a.codeSel) {
		a.codeSel, a.codeFont = defaultFontLabel, ""
	}
}

// persist 保存字体设置，失败时提示但不影响当前显示。
func (a *app) persist(c *ui.Context) {
	if err := a.savePrefs(); err != nil {
		c.Toast("字体设置未能保存：" + err.Error())
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func familyOf(label string) string {
	if label == defaultFontLabel {
		return ""
	}
	return label
}

// markdownTheme 只在主题、字体或缩放变化时重建，避免每帧替换指针导致整篇重排。
func (a *app) markdownTheme(c *ui.Context) {
	t := c.Theme()
	key := fmt.Sprint(t.Dark, t.Text, a.font, a.codeFont, a.fontSize, a.zoom)
	if a.md.Theme != nil && a.themeKey == key {
		return
	}
	th := markdown.DefaultTheme(t)
	if a.font != "" {
		th.Font = a.font
	}
	if a.codeFont != "" {
		th.CodeFont = a.codeFont
	}
	// 字号是其余长度的单位（标题、间距、代码块内边距都是 em），改它就整体缩放了正文。
	if a.fontSize > 0 {
		th.FontSize = float32(a.fontSize)
	}
	if a.zoom != defaultZoom {
		th.FontSize *= float32(a.zoom) / defaultZoom
	}
	th.LineHeight = bodyLineHeight
	// 标题比默认更粗一档，提升层级辨识。
	headingWeights := [6]int{800, 750, 700, 700, 700, 700}
	for i := range th.Headings {
		th.Headings[i].Weight = headingWeights[i]
	}
	a.md.Theme = th
	a.themeKey = key
}

// pickFile 在后台显示系统打开对话框，结果回到主线程。
func (a *app) pickFile() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win,
			Title:  "打开 Markdown 文件",
			Filters: []mygo.FileFilter{{
				Name:       "Markdown",
				Extensions: []string{"md", "markdown", "mdown", "mkd", "txt"},
			}},
		})
		if err != nil {
			a.win.Update(func() { a.showError("打开对话框失败：" + err.Error()) })
			return
		}
		if len(paths) == 0 {
			return
		}
		a.openPath(paths[0])
	}()
}

// openPath 读入文件并更新界面，可从任意 goroutine 调用。
func (a *app) openPath(path string) {
	src, err := readMarkdown(path)
	a.win.Update(func() {
		if err != nil {
			a.showError(fmt.Sprintf("无法打开 %s：%v", filepath.Base(path), err))
			return
		}
		a.path, a.src, a.err, a.notice = path, src, "", ""
		a.scroll.Y = 0
		a.win.SetTitle(filepath.Base(path) + " — " + appName)
	})
}

// readMarkdown 读取文件内容，大小写崩溃的边界在调用处处理。
func readMarkdown(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
