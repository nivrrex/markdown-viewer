package main

import (
	"math"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// 界面缩放的上下限，与 ui.Context.SetZoom 允许的范围一致。
const (
	uiScaleMin = 0.75
	uiScaleMax = 3
)

// uiScale 返回界面还需要放大多少，才能让内容 DIP 与桌面的 DPI 一致。
//
// Linux 上 GTK 只报告整数缩放系数，桌面把 Xft.dpi 设为 144 时它仍是 1，
// 内容就按 96 DPI 显示，字显得小。这里取 Xft.dpi 与 96 的比值，
// 再除以已经生效的整数缩放，避免 GDK_SCALE=2 时重复放大。
// 读不到 DPI 时返回 1，不改动界面。
func uiScale(monitorScale float64) float32 {
	return scaleFromDPI(xftDPI(), monitorScale)
}

// scaleFromDPI 是 uiScale 的纯计算部分，便于测试。
func scaleFromDPI(dpi, monitorScale float64) float32 {
	if dpi <= 0 {
		return 1
	}
	z := float32(dpi / 96)
	if monitorScale > 0 {
		z /= float32(monitorScale)
	}
	if z < uiScaleMin {
		return uiScaleMin
	}
	if z > uiScaleMax {
		return uiScaleMax
	}
	// 与 1 的差别小于半个百分点时不做缩放，避免无谓的重新布局。
	if math.Abs(float64(z)-1) < 0.005 {
		return 1
	}
	return z
}

// xftDPI 读取 X 资源里的 Xft.dpi，读不到返回 0。只在 Linux 上调用。
func xftDPI() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	out, err := exec.Command("xrdb", "-query").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "Xft.dpi" {
			continue
		}
		dpi, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || dpi <= 0 {
			return 0
		}
		return dpi
	}
	return 0
}
