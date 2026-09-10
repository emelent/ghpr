package gh

import (
	"encoding/json"
	"fmt"
	"time"
)

const reviewThreadsQuery = `
query($owner: String!, $name: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          isResolved
          isOutdated
          path
          line
          originalLine
          startLine
          diffSide
          comments(first: 100) {
            nodes {
              id
              databaseId
              body
              createdAt
              url
              author { login }
            }
          }
        }
      }
    }
  }
}`

type threadsResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						ID           string `json:"id"`
						IsResolved   bool   `json:"isResolved"`
						IsOutdated   bool   `json:"isOutdated"`
						Path         string `json:"path"`
						Line         *int   `json:"line"`
						OriginalLine *int   `json:"originalLine"`
						StartLine    *int   `json:"startLine"`
						DiffSide     string `json:"diffSide"`
						Comments     struct {
							Nodes []struct {
								ID         string    `json:"id"`
								DatabaseID int64     `json:"databaseId"`
								Body       string    `json:"body"`
								CreatedAt  time.Time `json:"createdAt"`
								URL        string    `json:"url"`
								Author     *struct {
									Login string `json:"login"`
								} `json:"author"`
							} `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// ReviewThreads fetches all review threads (with comments) for the PR.
func (c *Client) ReviewThreads(number int) ([]Thread, error) {
	owner, name := c.ownerName()
	if owner == "" {
		return nil, fmt.Errorf("repository must be owner/name, got %q", c.Repo)
	}
	var threads []Thread
	cursor := ""
	for {
		args := []string{"api", "graphql",
			"-f", "query=" + reviewThreadsQuery,
			"-f", "owner=" + owner,
			"-f", "name=" + name,
			"-F", fmt.Sprintf("number=%d", number),
		}
		if cursor != "" {
			args = append(args, "-f", "cursor="+cursor)
		}
		out, err := run(nil, args...)
		if err != nil {
			return nil, err
		}
		var resp threadsResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return nil, fmt.Errorf("decode review threads: %w", err)
		}
		if len(resp.Errors) > 0 {
			return nil, fmt.Errorf("graphql: %s", resp.Errors[0].Message)
		}
		rt := resp.Data.Repository.PullRequest.ReviewThreads
		for _, n := range rt.Nodes {
			t := Thread{
				ID:           n.ID,
				IsResolved:   n.IsResolved,
				IsOutdated:   n.IsOutdated,
				Path:         n.Path,
				Line:         deref(n.Line),
				OriginalLine: deref(n.OriginalLine),
				StartLine:    deref(n.StartLine),
				DiffSide:     n.DiffSide,
			}
			for _, cm := range n.Comments.Nodes {
				author := "ghost"
				if cm.Author != nil {
					author = cm.Author.Login
				}
				t.Comments = append(t.Comments, Comment{
					ID:         cm.ID,
					DatabaseID: cm.DatabaseID,
					Author:     author,
					Body:       cm.Body,
					CreatedAt:  cm.CreatedAt,
					URL:        cm.URL,
				})
			}
			threads = append(threads, t)
		}
		if !rt.PageInfo.HasNextPage {
			break
		}
		cursor = rt.PageInfo.EndCursor
	}
	return threads, nil
}

func (c *Client) mutateThread(mutation, threadID string) error {
	query := fmt.Sprintf(`mutation($id: ID!) { %s(input: {threadId: $id}) { thread { id isResolved } } }`, mutation)
	out, err := run(nil, "api", "graphql", "-f", "query="+query, "-f", "id="+threadID)
	if err != nil {
		return err
	}
	var resp struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(out, &resp); err == nil && len(resp.Errors) > 0 {
		return fmt.Errorf("graphql: %s", resp.Errors[0].Message)
	}
	return nil
}

// ResolveThread marks a review thread as resolved.
func (c *Client) ResolveThread(threadID string) error {
	return c.mutateThread("resolveReviewThread", threadID)
}

// UnresolveThread reopens a resolved review thread.
func (c *Client) UnresolveThread(threadID string) error {
	return c.mutateThread("unresolveReviewThread", threadID)
}
