name: Create Repository
description: Request a new repository to be created via IssueOps
title: "[REPO] Create: "
labels: ["issueops", "create-repo", "pending-approval"]
assignees:
  - platform-team-lead

body:
  - type: markdown
    attributes:
      value: |
        ## Create New Repository
        
        This issue will create a new repository in the organization.
        
        **Approval Required**: This request needs 2 approvals from `platform-team` members before it will be processed.
        
        **To Approve**: React with 👍 (thumbs up) or comment `approved` or `/approve`

  - type: input
    id: repo_name
    attributes:
      label: Repository Name
      description: Name of the repository (lowercase, hyphens only)
      placeholder: "my-awesome-service"
    validations:
      required: true

  - type: input
    id: workflow_file
    attributes:
      label: Workflow Filename
      description: Name of the empty workflow file to create (e.g., ci.yml)
      placeholder: "ci.yml"
    validations:
      required: true

  - type: dropdown
    id: team
    attributes:
      label: Responsible Team
      description: Which team will own this repository?
      options:
        - platform-team
        - data-team
        - engineering
        - security-team
        - devops-team
      default: 0
    validations:
      required: true

  - type: textarea
    id: description
    attributes:
      label: Repository Description
      description: Brief description of what this repository is for
      placeholder: "Service that handles authentication and authorization"
    validations:
      required: true

  - type: dropdown
    id: visibility
    attributes:
      label: Visibility
      description: Repository visibility setting
      options:
        - private
        - internal
        - public
      default: 0
    validations:
      required: true

  - type: checkboxes
    id: features
    attributes:
      label: Repository Features
      description: Select which features to enable
      options:
        - label: Issues
          required: false
        - label: Projects
          required: false
        - label: Wiki
          required: false
        - label: Discussions
          required: false

  - type: checkboxes
    id: requirements
    attributes:
      label: Requirements & Compliance
      options:
        - label: Require signed commits
          required: false
        - label: I have verified this repository name is not already in use
          required: true
        - label: I have approval from my team lead
          required: true
        - label: I understand this requires 2 platform-team approvals
          required: true

  - type: markdown
    attributes:
      value: |
        ---
        
        ### What happens next?
        
        1. **Approval Phase**: 2 members of `platform-team` must approve (👍 or comment `/approve`)
        2. **Automation**: Once approved, the workflow will:
           - Update `repo-yamls/platform-repos.yml` with new repository
           - Create branch `request/issue-{number}`
           - Open PR for review
        3. **Terraform**: Once PR is merged, Terraform will create the actual repository
        
        **Questions?** Ping @platform-team in #infrastructure on Slack
