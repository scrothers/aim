// Package prompts registers AIM's user-invocable MCP prompts. Each prompt is a
// workflow script in markdown/<name>.md (front matter plus a text/template
// body) with its Go behavior, such as default arguments and attached mail, in
// <name>.go.
package prompts
