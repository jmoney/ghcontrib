package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

type repo struct {
	NameWithOwner string
	URL           string
	IsPrivate     bool
}

type pullRequestContributionQuery struct {
	User struct {
		ContributionsCollection struct {
			PullRequestContributions struct {
				PageInfo struct {
					HasNextPage bool
					EndCursor   githubv4.String
				}
				Nodes []struct {
					PullRequest struct {
						Repository repo
					}
				}
			} `graphql:"pullRequestContributions(first: 100, after: $cursor)"`
		} `graphql:"contributionsCollection(from: $from, to: $to)"`
	} `graphql:"user(login: $login)"`
}

type commitContributionQuery struct {
	User struct {
		ContributionsCollection struct {
			CommitContributionsByRepository []struct {
				Repository repo
			} `graphql:"commitContributionsByRepository(maxRepositories: 100)"`
		} `graphql:"contributionsCollection(from: $from, to: $to)"`
	} `graphql:"user(login: $login)"`
}

func main() {
	// CLI flags
	username := flag.String("username", "", "GitHub username")
	startYear := flag.Int("start", 2020, "Start year (inclusive)")
	endYear := flag.Int("end", time.Now().Year(), "End year (inclusive)")
	flag.Parse()

	if *username == "" {
		log.Fatal("Missing required flag: --username")
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is not set")
	}

	src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(context.Background(), src)
	client := githubv4.NewClient(httpClient)

	reposByYear := make(map[int]map[string]string)

	for year := *startYear; year <= *endYear; year++ {
		from := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)

		repos := make(map[string]string)

		// Paginate pull request contributions
		var prCursor *githubv4.String
		for {
			var q pullRequestContributionQuery
			variables := map[string]interface{}{
				"login":  githubv4.String(*username),
				"from":   githubv4.DateTime{Time: from},
				"to":     githubv4.DateTime{Time: to},
				"cursor": prCursor,
			}

			err := client.Query(context.Background(), &q, variables)
			if err != nil {
				log.Fatalf("Pull request query failed for year %d: %v", year, err)
			}

			for _, node := range q.User.ContributionsCollection.PullRequestContributions.Nodes {
				repo := node.PullRequest.Repository
				if !repo.IsPrivate && !strings.HasPrefix(repo.NameWithOwner, *username+"/") {
					repos[repo.NameWithOwner] = repo.URL
				}
			}

			if !q.User.ContributionsCollection.PullRequestContributions.PageInfo.HasNextPage {
				break
			}
			prCursor = &q.User.ContributionsCollection.PullRequestContributions.PageInfo.EndCursor
		}

		// Commit contributions (no pagination)
		var cq commitContributionQuery
		commitVars := map[string]interface{}{
			"login": githubv4.String(*username),
			"from":  githubv4.DateTime{Time: from},
			"to":    githubv4.DateTime{Time: to},
		}
		err := client.Query(context.Background(), &cq, commitVars)
		if err != nil {
			log.Fatalf("Commit query failed for year %d: %v", year, err)
		}

		for _, node := range cq.User.ContributionsCollection.CommitContributionsByRepository {
			repo := node.Repository
			if !repo.IsPrivate && !strings.HasPrefix(repo.NameWithOwner, *username+"/") {
				repos[repo.NameWithOwner] = repo.URL
			}
		}

		if len(repos) > 0 {
			reposByYear[year] = repos
		}
	}

	// Output JSON
	jsonOutput, err := json.Marshal(reposByYear)
	if err != nil {
		log.Fatalf("Failed to encode JSON: %v", err)
	}

	fmt.Println(string(jsonOutput))
}
