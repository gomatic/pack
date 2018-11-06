package style

import "github.com/fatih/color"

var Help = color.CyanString

var Tip = color.New(color.FgHiGreen, color.Bold).SprintfFunc()

var Error = color.New(color.FgRed, color.Bold).SprintfFunc()

var Separator = color.HiCyanString
