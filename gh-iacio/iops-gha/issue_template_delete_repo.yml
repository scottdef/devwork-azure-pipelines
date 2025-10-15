name: Delete Repository
description: Request repository deletion via IssueOps
title: "[REPO] Delete: "
labels: ["issueops", "delete-repo", "pending-approval", "destructive"]
assignees:
  - platform-team-lead

body:
  - type: markdown
    attributes:
      value: |
        ## ⚠️ Delete Repository
        
        **WARNING**: This is a destructive operation. The repository will be removed from YAML files.
        The actual GitHub repository deletion must be done manually for safety.
        
        **Approval Required**: This request needs 2 approvals from `platform-team` members.
        
        **To Approve**: React with 👍 or comment `approved` or `/approve`

  - type: input
    id: repo_name
    attributes:
      label: Repository Name
      description: Exact name of the repository to delete
      placeholder: "my-old-service"
    validations:
      required: true

  - type: dropdown
    id: team
    attributes:
      label: Repository Team
      description: Which team owns this repository?
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
    id: reason
    attributes:
      label: Reason for Deletion
      description: Why is this repository being deleted?
      placeholder: "No longer in use, migrated to new-service, project cancelled, etc."
    validations:
      required: true

  - type: checkboxes
    id: confirmations
    attributes:
      label: Confirmations
      description: You must check all boxes to proceed
      options:
        - label: I have backed up any important data from this repository
          required: true
        - label: I have verified this repository is not in active use
          required: true
        - label: I have communicated this deletion to the team
          required: true
        - label: I understand the repository will be removed from YAML but must be manually deleted from GitHub
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
        
        1. **Approval Phase**: 2 members of `platform-team` must approve
        2. **Automation**: Removes repository from YAML files and creates PR
        3. **Manual Step**: After PR merge, you must manually delete the GitHub repository
        4. **Run**: `gh repo delete OWNER/REPO-NAME` or use GitHub web UI
        
        **The automation does NOT delete the actual GitHub repository for safety.**
        
        **Questions?** Ping @platform-team in #infrastructure on Slack
