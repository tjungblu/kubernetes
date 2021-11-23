package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"
)

var (
	// AllCommitValidators holds all registered checks.
	AllCommitValidators = []func(Commit) []string{
		ValidateCommitAuthor,

		// Local commit messages must be prefixed with UPSTREAM as per
		// README.openshift.md to aid in rebasing on upstream kube.
		ValidateCommitMessage,
	}
)

// ValidateReleaseBranchConsistency outputs an error in case we're missing a commit in
// any of the following (n+2) release branches. This helps us to understand if we're merging a fix that isn't present
// in later versions of OCP yet - which might cause regression problems in cluster upgrades.
// This currently only works on release-4.x branches for simplicity.
func ValidateReleaseBranchConsistency(commits []Commit) (allErrors []string) {
	branch, err := CurrentBranch()
	if err != nil {
		return append(allErrors, fmt.Sprintf("Could not retrieve branch information: %v", err))
	}

	if !IsReleaseBranch(branch) {
		return allErrors
	}

	version, err := ReleaseBranchYVersion(branch)
	if err != nil {
		return append(allErrors, fmt.Sprintf("Could not retrieve y version information: %v", err))
	}

	releaseBranches := []string{
		fmt.Sprintf("release-4.%d", version+1),
		fmt.Sprintf("release-4.%d", version+2),
	}

	for _, branch := range releaseBranches {
		err := FetchBranch(branch)
		if err == nil {
			for _, commit := range commits {
				if IsCommitMissingInBranch(commit, branch) {
					allErrors = append(allErrors, fmt.Sprintf("Commit %s - '%s' is missing in branch %s", commit.Sha, commit.Summary, branch))
				}
			}
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "error fetching release branch %s: %v\n", branch, err)
		}
	}

	return allErrors
}

func ValidateCommitAuthor(commit Commit) []string {
	var allErrors []string

	if strings.HasPrefix(commit.Email, "root@") {
		allErrors = append(allErrors, fmt.Sprintf("Commit %s has invalid email %q", commit.Sha, commit.Email))
	}

	return allErrors
}

func ValidateCommitMessage(commit Commit) []string {
	if commit.MatchesMergeSummaryPattern() {
		// Ignore merges
		return nil
	}

	var allErrors []string

	if !commit.MatchesUpstreamSummaryPattern() {
		tmpl, _ := template.New("problems").Parse(`
UPSTREAM commit {{ .Commit.Sha }} has invalid summary {{ .Commit.Summary }}.

UPSTREAM commits are validated against the following regular expression:
  {{ .Pattern }}

UPSTREAM commit summaries should look like:

  UPSTREAM: <PR number|carry|drop>: description

UPSTREAM commits which revert previous UPSTREAM commits should look like:

  UPSTREAM: revert: <normal upstream format>

Examples of valid summaries:

  UPSTREAM: 12345: A kube fix
  UPSTREAM: <carry>: A carried kube change
  UPSTREAM: <drop>: A dropped kube change
  UPSTREAM: revert: 12345: A kube revert
`)
		data := struct {
			Pattern *regexp.Regexp
			Commit  Commit
		}{
			Pattern: UpstreamSummaryPattern,
			Commit:  commit,
		}
		buffer := &bytes.Buffer{}
		err := tmpl.Execute(buffer, data)
		if err != nil {
			allErrors = append(allErrors, err.Error())
			return allErrors
		}

		allErrors = append(allErrors, buffer.String())

		return allErrors
	}

	return allErrors
}
