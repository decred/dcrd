### All Submissions:

* [ ] Submission follows the [Code Contribution Guidelines](../blob/master/docs/code_contribution_guidelines.md)
* [ ] There are not any other open [Pull Requests](../pulls) for the same update/change
* [ ] Commit messages are formatted according to [Model Git Commit Messages](../blob/master/docs/code_contribution_guidelines.md#44-model-git-commit-messages)
* [ ] All changes are compliant with the latest version of Go and the one prior to it
* [ ] The code being submitted is commented according to the [Code Documentation and Commenting](../blob/master/docs/code_contribution_guidelines.md#CodeDocumentation) section of the Code Contribution Guidelines
* [ ] Any new logging statements use an appropriate subsystem and logging level
* [ ] Code has been formatted with go fmt
* [ ] Running go test does not fail any tests or report any vet issues
* [ ] Running golint does not report any new issues that did not already exist

### New Feature Submissions:

* [ ] Code is accompanied by tests which exercise both the positive and negative (error paths) conditions (if applicable)

### Bug Fixes:

* [ ] Code is accompanied by new tests which trigger the bug being fixed to prevent regressions

### Changes to Core Features:

* [ ] An explanation of what the changes do and why they should be included is provided
* [ ] Code is accompanied by updates to tests and/or new tests for the core changes (if applicable)
