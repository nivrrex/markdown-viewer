# MarkdownViewer

一个用 Go 写的桌面 Markdown 阅读器。界面由 MyGo 在 GPU 上直接绘制，不使用 WebView、不使用 CGO，整个应用是一个单文件二进制。

## 功能

- **Markdown 渲染**：CommonMark，加 GitHub 的表格、任务列表、删除线、脚注。
- **代码块**：语法高亮，右上角带复制按钮，复制出的是纯代码文本。
- **字体与字号**：正文字体、代码字体从系统已安装的字体中选择，正文字号 10–28 DIP 可调，选择与字号都会保存，下次启动继续使用。
- **字号缩放**：`Ctrl` + 鼠标滚轮按 10% 一档缩放正文，也可用工具栏的 `−` / `+` 按钮，范围 50%–250%；点中间的比例可一键回到 100%。
- **内容宽度**：`60%`、`80%`、`100%` 三档按窗口可用宽度缩放，默认 `80%`。
- **导出 PDF**：在后台生成适合 A4 的打印视图，以原 Markdown 所在目录为基址解析相对图片与链接，再交给系统默认浏览器，由浏览器的「打印 → 另存为 PDF」完成导出。
- **打开方式**：工具栏的「打开…」按钮，或命令行参数。

Windows 上默认正文为微软雅黑、16 DIP，Linux 跟随系统界面字体、14 DIP，行高 1.6；这些都可在字体面板里改并保存。

## 界面

工具栏浮在正文之上，启动时显示 3 秒后自动隐藏；指针移到窗口顶部约 8 DIP 内会再次出现，指针离开窗口内容区或工具栏 1.5 秒后隐藏。工具栏换行后只要指针仍在其上就保持显示，字体面板打开时同样保持。正文因此始终占满窗口宽度。

## 高 DPI

Linux 上读取桌面的 `Xft.dpi`（`xrdb -query`），按它与 96 的比值缩放整个界面，和浏览器缩放页面一样按更高像素密度绘制，字形不会因此发糊。已经生效的整数缩放（如 `GDK_SCALE=2`）会从比值里除掉，避免重复放大；读不到 `Xft.dpi` 时按 96 处理，不缩放。

Windows 直接使用系统上报的缩放系数。

## 系统要求

| 平台 | 要求 |
|---|---|
| Linux | GTK 3（`libgtk-3-0`）；不需要 WebKitGTK |
| Windows | Windows 10 / 11；不需要任何运行时 |

## 安装

### Debian / Ubuntu

```bash
sudo dpkg -i markdown-viewer_0.1.0_amd64.deb
```

安装后可从应用菜单启动，或在终端运行 `markdown-viewer`。

### 其他 Linux 发行版

下载 `markdown-viewer-0.1.0-linux-amd64.tar.gz` 解压后运行其中的可执行文件，或使用包内的 `install.sh` 安装到当前用户目录。

### Windows

下载 `markdown-viewer-0.1.0-windows-amd64.exe`，双击运行，无需安装。

## 使用

```bash
markdown-viewer 文档.md
```

不带参数启动时，用工具栏的「打开…」选择 `.md` 文件。

- **宽度**：工具栏中间的 `60%` `80%` `100%` 切换阅读栏宽度，默认 `80%`。
- **字体**：点「字体」展开面板，选择正文字体、代码字体，并输入正文字号（10–28 DIP，默认 14）。
- **缩放**：`Ctrl` + 滚轮，或宽度档位右侧的 `−` / `100%` / `+`。缩放只影响正文，不影响工具栏；无 `Ctrl` 的滚轮照常滚动正文。
- **导出 PDF**：点「导出 PDF」，后台生成打印视图后由浏览器打开，用浏览器的打印功能保存为 PDF。相对路径的图片按原 Markdown 所在目录解析。

字体与字号保存在用户数据目录下的 `settings.json`（Linux 为 `~/.config/MarkdownViewer/settings.json`，Windows 为 `%AppData%\MarkdownViewer\settings.json`）。删除该文件即可恢复默认。

## 从源码构建

需要 Go 1.27.4 或更高版本。

```bash
git clone https://github.com/nivrrex/markdown-viewer.git
cd markdown-viewer
go mod download github.com/egoist/mygo     # 只首次，把 mygo 取到本机模块缓存
sh scripts/setup-mygo-fork.sh              # 复制 mygo v0.10.0 并打上文本渲染补丁
go tool mygo build -platform linux/amd64,windows/amd64 -skip-installer
```

这一步不可省略。脚本使用本机 Go 工具链（`GOTOOLCHAIN=local`，可用 `GO=/path/to/go` 指定可执行文件），模块缓存里没有 mygo 时会自行下载；重复运行只验证补丁状态，不覆盖已有 fork。应用使用 mygo v0.10.0，其中 Windows 路径默认对字形启用 ClearType 子像素覆盖，中文小字号会出现红蓝彩边，观感偏颗粒；`patches/mygo-cleartype-grayscale.patch` 把 Windows 路径改为灰度抗锯齿，与 Linux 的 Pango、Chromium 的 GPU 合成文本一致。`go.mod` 通过 `replace` 指向脚本生成的 `.mygo-fork/mygo`，该目录由 `.gitignore` 忽略。脚本可重复执行，删除 `.mygo-fork` 后重跑即可重建。

产物在 `build/` 下：Linux 可执行文件、`.deb`、`.rpm`、`.tar.gz`、`PKGBUILD`，以及 Windows 的 `.exe`。`-skip-installer` 只跳过需要 NSIS 的 Windows 安装程序，不影响 `.exe`。

开发时热重载：

```bash
go tool mygo dev
```

## 项目结构

```
main.go        应用状态、视图、文件打开、宽度档位、字号缩放、工具栏自动隐藏
prefs.go       字体与字号的读写
dpi.go         读取 Xft.dpi 并换算界面缩放
print.go       PDF 导出的打印视图与浏览器调用
main_test.go   无窗口测试
mygo.json      应用元数据与打包配置
resources/     应用图标
patches/       mygo 的本地补丁（Windows 文本渲染）
scripts/       生成打过补丁的 mygo fork
```

## 测试

测试不打开窗口，直接在内存中渲染视图并模拟点击：

```bash
go test ./...
```

## 许可证

MIT，见 [LICENSE](LICENSE)。
