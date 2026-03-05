package main

import (
	"fmt"
	"os"
)

const (
	ColorReset  = "\033[0m"
	ColorBold   = "\033[1m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[90m"
)

var useColors = true

func init() {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		useColors = false
	}
}

func colorize(color, text string) string {
	if !useColors {
		return text
	}
	return color + text + ColorReset
}

func Bold(text string) string {
	return colorize(ColorBold, text)
}

func Red(text string) string {
	return colorize(ColorRed, text)
}

func Green(text string) string {
	return colorize(ColorGreen, text)
}

func Yellow(text string) string {
	return colorize(ColorYellow, text)
}

func Blue(text string) string {
	return colorize(ColorBlue, text)
}

func Purple(text string) string {
	return colorize(ColorPurple, text)
}

func Cyan(text string) string {
	return colorize(ColorCyan, text)
}

func Gray(text string) string {
	return colorize(ColorGray, text)
}

func Success(text string) string {
	return Green("✓ " + text)
}

func Error(text string) string {
	return Red("✗ " + text)
}

func Info(text string) string {
	return Cyan("ℹ " + text)
}

func Warning(text string) string {
	return Yellow("⚠ " + text)
}

func Header(text string) string {
	return Bold(Cyan(text))
}

func Highlight(text string) string {
	return Bold(Yellow(text))
}

func Dim(text string) string {
	return Gray(text)
}

func printSuccess(format string, args ...interface{}) {
	fmt.Printf(Success(format)+"\n", args...)
}

func printError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, Error(format)+"\n", args...)
}

func printInfo(format string, args ...interface{}) {
	fmt.Printf(Info(format)+"\n", args...)
}

func printWarning(format string, args ...interface{}) {
	fmt.Printf(Warning(format)+"\n", args...)
}
