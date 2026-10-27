// Package claude asks the Claude Code CLI (`claude -p`) questions about the
// code under review. It reuses the user's Claude Code login, so ghpr needs
// no API key of its own.
package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultBin is the program run when no other is configured.
const DefaultBin = "claude"

// Tools are the tools Claude may use in a checkout of the code under
// review: read-only, so it can look at the surrounding code but never change
// it. Without a checkout it gets none.
const Tools = "Read,Grep,Glob"

// SystemPrompt is appended to Claude Code's own system prompt.
const SystemPrompt = "You are helping someone review a GitHub pull request in a terminal UI. " +
	"They selected lines from the diff and ask about them. Answer concisely in plain text " +
	"(the answer is shown in a narrow terminal panel; avoid tables and headings). " +
	"Lines starting with + were added, - removed, a space unchanged."

// execCommand builds the command run by Ask; tests replace it.
var execCommand = exec.Command

// lookPath finds the binary for Available; tests replace it.
var lookPath = exec.LookPath

// Answer is one reply and the session it belongs to, which a follow-up
// passes back to continue the conversation.
type Answer struct {
	Text      string
	SessionID string
}

// Available reports whether bin can be run.
func Available(bin string) bool {
	_, err := lookPath(bin)
	return err == nil
}

// Args builds the command line for one question; sessionID, when set,
// resumes that conversation and model, when set, picks the model. readFiles
// grants the read-only Tools, otherwise every tool is off.
func Args(model, sessionID string, readFiles bool) []string {
	tools := ""
	if readFiles {
		tools = Tools
	}
	args := []string{"-p", "--output-format", "json", "--tools", tools, "--append-system-prompt", SystemPrompt}
	if model != "" {
		args = append(args, "--model", model)
	}
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	return args
}

// result is the part of `claude -p --output-format json` that ghpr reads.
type result struct {
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
	IsError   bool   `json:"is_error"`
}

// Ask sends prompt to bin on stdin and returns the answer. dir, when set, is
// a checkout of the code under review: Claude runs there and may read its
// files. Follow-ups must pass the same dir, as Claude Code keeps sessions
// per directory.
func Ask(bin, model, sessionID, dir, prompt string) (Answer, error) {
	if bin == "" {
		bin = DefaultBin
	}
	cmd := execCommand(bin, Args(model, sessionID, dir != "")...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	var r result
	if err := json.Unmarshal(stdout.Bytes(), &r); err != nil {
		if runErr != nil {
			return Answer{}, fmt.Errorf("%s: %s", bin, firstLine(stderr.String(), stdout.String(), runErr.Error()))
		}
		return Answer{}, fmt.Errorf("%s: unreadable output: %w", bin, err)
	}
	if r.IsError || runErr != nil {
		msg := r.Result
		if runErr != nil {
			msg = firstLine(r.Result, stderr.String(), runErr.Error())
		}
		return Answer{}, errors.New(firstLine(msg, "claude reported an error"))
	}
	return Answer{Text: strings.TrimSpace(r.Result), SessionID: r.SessionID}, nil
}

// firstLine returns the first line of the first non-blank string.
func firstLine(ss ...string) string {
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			s, _, _ = strings.Cut(s, "\n")
			return s
		}
	}
	return ""
}
