// Package gh wraps the GitHub CLI (`gh`) to fetch pull request data and
// perform review actions. Every call shells out to `gh`, so the user's
// existing authentication and host configuration are reused.
package gh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Client performs operations against a single repository.
type Client struct {
	// Repo is "owner/name".
	Repo string
}

// PR is the detailed view of a pull request.
type PR struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	State          string `json:"state"`
	URL            string `json:"url"`
	BaseRefName    string `json:"baseRefName"`
	HeadRefName    string `json:"headRefName"`
	HeadRefOid     string `json:"headRefOid"`
	IsDraft        bool   `json:"isDraft"`
	ReviewDecision string `json:"reviewDecision"`
	// Mergeable is MERGEABLE, CONFLICTING or UNKNOWN; MergeStateStatus is
	// CLEAN, BLOCKED, BEHIND, DIRTY, UNSTABLE, HAS_HOOKS, DRAFT or UNKNOWN.
	Mergeable        string `json:"mergeable"`
	MergeStateStatus string `json:"mergeStateStatus"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	ChangedFiles     int    `json:"changedFiles"`
	Author           struct {
		Login string `json:"login"`
	} `json:"author"`
	HeadRepoOwner string `json:"-"`
}

// PRSummary is a row in a pull request listing.
type PRSummary struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	State          string    `json:"state"` // OPEN, CLOSED or MERGED
	HeadRefName    string    `json:"headRefName"`
	IsDraft        bool      `json:"isDraft"`
	ReviewDecision string    `json:"reviewDecision"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Author         struct {
		Login string `json:"login"`
	} `json:"author"`
}

// Thread is a review thread anchored to a diff line.
type Thread struct {
	ID         string
	IsResolved bool
	IsOutdated bool
	Path       string
	// Line is the current line number on DiffSide, or 0 when the thread is
	// outdated and no longer maps onto the diff.
	Line         int
	OriginalLine int
	StartLine    int
	DiffSide     string // "LEFT" or "RIGHT"
	Comments     []Comment
}

// Comment is a single review comment within a thread.
type Comment struct {
	ID         string
	DatabaseID int64
	Author     string
	Body       string
	CreatedAt  time.Time
	URL        string
}

// Error is returned when gh exits non-zero.
type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("gh %s: %s", strings.Join(e.Args, " "), msg)
}

func run(stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, &Error{Args: args, Stderr: stderr.String(), Err: err}
	}
	return stdout.Bytes(), nil
}

// CurrentRepo resolves the repository for the current working directory.
func CurrentRepo() (string, error) {
	out, err := run(nil, "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

var prURLRe = regexp.MustCompile(`github\.com/([^/]+/[^/#]+)/pull/(\d+)`)
var prShortRe = regexp.MustCompile(`^([^/#\s]+/[^/#\s]+)#(\d+)$`)

// ParsePRRef accepts "123", "#123", a PR URL or "owner/repo#123" and returns
// the repository (possibly empty) and PR number.
func ParsePRRef(ref string) (repo string, number int, err error) {
	ref = strings.TrimSpace(ref)
	if m := prURLRe.FindStringSubmatch(ref); m != nil {
		n, _ := strconv.Atoi(m[2])
		return strings.TrimSuffix(m[1], ".git"), n, nil
	}
	if m := prShortRe.FindStringSubmatch(ref); m != nil {
		n, _ := strconv.Atoi(m[2])
		return m[1], n, nil
	}
	n, err := strconv.Atoi(strings.TrimPrefix(ref, "#"))
	if err != nil {
		return "", 0, fmt.Errorf("cannot parse pull request reference %q", ref)
	}
	return "", n, nil
}

func (c *Client) repoArgs() []string {
	if c.Repo == "" {
		return nil
	}
	return []string{"-R", c.Repo}
}

func (c *Client) ownerName() (string, string) {
	parts := strings.SplitN(c.Repo, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// ListPRs returns pull requests in the given state: "open", "closed",
// "merged" or "all".
func (c *Client) ListPRs(state string, limit int) ([]PRSummary, error) {
	if state == "" {
		state = "open"
	}
	args := append([]string{"pr", "list", "--state", state, "--limit", strconv.Itoa(limit),
		"--json", "number,title,state,author,headRefName,isDraft,reviewDecision,updatedAt"}, c.repoArgs()...)
	out, err := run(nil, args...)
	if err != nil {
		return nil, err
	}
	var prs []PRSummary
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("decode pr list: %w", err)
	}
	return prs, nil
}

// ViewPR fetches PR metadata.
func (c *Client) ViewPR(number int) (*PR, error) {
	args := append([]string{"pr", "view", strconv.Itoa(number),
		"--json", "number,title,body,state,url,baseRefName,headRefName,headRefOid,isDraft,reviewDecision,mergeable,mergeStateStatus,additions,deletions,changedFiles,author,headRepositoryOwner"},
		c.repoArgs()...)
	out, err := run(nil, args...)
	if err != nil {
		return nil, err
	}
	var pr PR
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, fmt.Errorf("decode pr: %w", err)
	}
	var owner struct {
		HeadRepositoryOwner struct {
			Login string `json:"login"`
		} `json:"headRepositoryOwner"`
	}
	if json.Unmarshal(out, &owner) == nil {
		pr.HeadRepoOwner = owner.HeadRepositoryOwner.Login
	}
	return &pr, nil
}

// Diff returns the unified diff for the PR.
func (c *Client) Diff(number int) (string, error) {
	args := append([]string{"pr", "diff", strconv.Itoa(number)}, c.repoArgs()...)
	out, err := run(nil, args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// FileContent fetches the raw content of a file at a given ref (branch or
// commit SHA).
func (c *Client) FileContent(ref, path string) (string, error) {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	endpoint := fmt.Sprintf("repos/%s/contents/%s?ref=%s", c.Repo, strings.Join(segs, "/"), url.QueryEscape(ref))
	out, err := run(nil, "api", "-H", "Accept: application/vnd.github.raw+json", endpoint)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// OpenInBrowser opens the PR in the default browser.
func (c *Client) OpenInBrowser(number int) error {
	args := append([]string{"pr", "view", strconv.Itoa(number), "--web"}, c.repoArgs()...)
	_, err := run(nil, args...)
	return err
}

// ReviewEvent is the kind of review to submit.
type ReviewEvent string

const (
	Approve        ReviewEvent = "approve"
	RequestChanges ReviewEvent = "request-changes"
	CommentReview  ReviewEvent = "comment"
)

// Review submits a pull request review.
func (c *Client) Review(number int, event ReviewEvent, body string) error {
	args := append([]string{"pr", "review", strconv.Itoa(number), "--" + string(event)}, c.repoArgs()...)
	var stdin []byte
	if strings.TrimSpace(body) != "" {
		args = append(args, "--body-file", "-")
		stdin = []byte(body)
	}
	_, err := run(stdin, args...)
	return err
}

// MergeMethod selects how a PR is merged.
type MergeMethod string

const (
	MergeCommit MergeMethod = "merge"
	Squash      MergeMethod = "squash"
	Rebase      MergeMethod = "rebase"
)

// MergeOptions controls how a PR is merged. Subject and Body override the
// commit message for merge-commit and squash merges; empty values leave
// GitHub's defaults in place. Rebase merges have no commit message.
type MergeOptions struct {
	Method       MergeMethod
	DeleteBranch bool
	Subject      string
	Body         string
}

// Merge merges the pull request.
func (c *Client) Merge(number int, o MergeOptions) error {
	args := append([]string{"pr", "merge", strconv.Itoa(number), "--" + string(o.Method)}, c.repoArgs()...)
	if o.DeleteBranch {
		args = append(args, "--delete-branch")
	}
	var stdin []byte
	if o.Method != Rebase {
		if s := strings.TrimSpace(o.Subject); s != "" {
			args = append(args, "--subject", s)
		}
		if b := strings.TrimSpace(o.Body); b != "" {
			args = append(args, "--body-file", "-")
			stdin = []byte(b)
		}
	}
	_, err := run(stdin, args...)
	return err
}

// DefaultMergeMessage returns GitHub's default commit subject and body for a
// merge-commit or squash merge of the PR.
func DefaultMergeMessage(pr *PR, method MergeMethod) (subject, body string) {
	switch method {
	case Squash:
		return fmt.Sprintf("%s (#%d)", pr.Title, pr.Number), strings.TrimSpace(pr.Body)
	case MergeCommit:
		head := pr.HeadRefName
		if pr.HeadRepoOwner != "" {
			head = pr.HeadRepoOwner + "/" + head
		}
		return fmt.Sprintf("Merge pull request #%d from %s", pr.Number, head), pr.Title
	}
	return "", ""
}

// Close closes the pull request without merging, optionally deleting the
// head branch.
func (c *Client) Close(number int, deleteBranch bool) error {
	args := append([]string{"pr", "close", strconv.Itoa(number)}, c.repoArgs()...)
	if deleteBranch {
		args = append(args, "--delete-branch")
	}
	_, err := run(nil, args...)
	return err
}

// Reopen reopens a closed (not merged) pull request.
func (c *Client) Reopen(number int) error {
	args := append([]string{"pr", "reopen", strconv.Itoa(number)}, c.repoArgs()...)
	_, err := run(nil, args...)
	return err
}

// Comment posts a general (non-review) comment on the PR.
func (c *Client) Comment(number int, body string) error {
	args := append([]string{"pr", "comment", strconv.Itoa(number), "--body-file", "-"}, c.repoArgs()...)
	_, err := run([]byte(body), args...)
	return err
}

// LineComment describes where a new review thread should be anchored.
// Line/Side is the (last) line of the comment. StartLine/StartSide, when
// StartLine > 0, make it a multi-line comment spanning StartLine..Line.
type LineComment struct {
	Path      string
	Line      int
	Side      string // "LEFT" or "RIGHT"
	StartLine int
	StartSide string
	Body      string
}

// IsRange reports whether the comment spans more than one line.
func (lc LineComment) IsRange() bool {
	return lc.StartLine > 0 && (lc.StartLine != lc.Line || lc.StartSide != lc.Side)
}

// AddLineComment creates a new review thread on a diff line or line range.
// commitID is the PR head SHA.
func (c *Client) AddLineComment(number int, commitID string, lc LineComment) error {
	fields := map[string]any{
		"body":      lc.Body,
		"commit_id": commitID,
		"path":      lc.Path,
		"line":      lc.Line,
		"side":      lc.Side,
	}
	if lc.IsRange() {
		fields["start_line"] = lc.StartLine
		fields["start_side"] = lc.StartSide
		if fields["start_side"] == "" {
			fields["start_side"] = lc.Side
		}
	}
	payload, _ := json.Marshal(fields)
	endpoint := fmt.Sprintf("repos/%s/pulls/%d/comments", c.Repo, number)
	_, err := run(payload, "api", "-X", "POST", endpoint, "--input", "-")
	return err
}

// CurrentUser returns the login of the authenticated gh user.
func (c *Client) CurrentUser() (string, error) {
	out, err := run(nil, "api", "user", "-q", ".login")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// DeleteReviewComment deletes a review comment (by REST id). Only the
// author or a repository admin may do so; GitHub rejects it otherwise.
func (c *Client) DeleteReviewComment(commentID int64) error {
	endpoint := fmt.Sprintf("repos/%s/pulls/comments/%d", c.Repo, commentID)
	_, err := run(nil, "api", "-X", "DELETE", endpoint)
	return err
}

// EditReviewComment replaces the body of a review comment (by REST id).
// Only the author may do so; GitHub rejects it otherwise.
func (c *Client) EditReviewComment(commentID int64, body string) error {
	payload, _ := json.Marshal(map[string]any{"body": body})
	endpoint := fmt.Sprintf("repos/%s/pulls/comments/%d", c.Repo, commentID)
	_, err := run(payload, "api", "-X", "PATCH", endpoint, "--input", "-")
	return err
}

// ReplyToComment replies to an existing review comment (by REST id).
func (c *Client) ReplyToComment(number int, commentID int64, body string) error {
	payload, _ := json.Marshal(map[string]any{"body": body})
	endpoint := fmt.Sprintf("repos/%s/pulls/%d/comments/%d/replies", c.Repo, number, commentID)
	_, err := run(payload, "api", "-X", "POST", endpoint, "--input", "-")
	return err
}
