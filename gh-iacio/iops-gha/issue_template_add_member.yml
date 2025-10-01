name: Add Team Member
description: Add a member to a team via IssueOps
title: "[TEAM] Add Member: "
labels: ["issueops", "add-member", "pending-approval"]
assignees:
  - platform-team-lead

body:
  - type: markdown
    attributes:
      value: |
        ## Add Team Member
        
        This will add a member to an existing team in the organization.
        
        **Approval Required**: This request needs 2 approvals from `platform-team` members.
        
        **To Approve**: React with 👍 or comment `approved` or `/approve`

  - type: input
    id: github_username
    attributes:
      label: GitHub Username
      description: The GitHub username of the person to add (without @)
      placeholder: "alice"
    validations:
      required: true

  - type: dropdown
    id: team
    attributes:
      label: Team
      description: Which team should this person join?
      options:
        - platform-team
        - data-team
        - engineering
        - security-team
        - devops-team
      default: 0
    validations:
      required: true

  - type: dropdown
    id: role
    attributes:
      label: Role
      description: What role should they have?
      options:
        - member
        - maintainer
      default: 0
    validations:
      required: true

  - type: textarea
    id: justification
    attributes:
      label: Justification
      description: Why should this person be added to the team?
      placeholder: "New hire on platform team, needs access to platform repositories"
    validations:
      required: true

  - type: checkboxes
    id: confirmations
    attributes:
      label: Confirmations
      options:
        - label: I have verified this GitHub username is correct
          required: true
        - label: I have approval from the team lead
          required: true
        - label: This person has completed onboarding requirements
          required: true
        - label: I understand this requires 2 platform-team approvals
          required: true

  - type: markdown
    attributes:
      value: |
        ---
        
        ### What happens next?
        
        1. **Approval Phase**: 2 members of `platform-team` must approve
        2. **Automation**: Updates team YAML file and creates PR
        3. **Terraform**: Once PR is merged, user is added to team
        4. **Access**: User gets access to team repositories automatically
        
        ### Role Differences
        
        - **Member**: Can view, clone, and push to team repositories
        - **Maintainer**: Members permissions + can manage team settings and membership
        
        **Questions?** Ping @platform-team in #infrastructure on Slack
