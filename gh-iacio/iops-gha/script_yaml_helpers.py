#!/usr/bin/env python3
"""
yaml-helpers.py - YAML manipulation helpers for IssueOps
Clean, simple, functional. The Unix way.
"""

import sys
import yaml
import json
from pathlib import Path
from typing import Dict, List, Optional


class YAMLHelper:
    """Helper for manipulating GitHub governance YAML files"""
    
    def __init__(self, root_path: Path):
        self.root = root_path
        self.repo_dir = root_path / "repo-yamls"
        self.team_dir = root_path / "team-yamls"
    
    def add_repo(self, team: str, repo_data: Dict) -> Path:
        """Add a repository to team's YAML file"""
        yaml_file = self.repo_dir / f"{team}-repos.yml"
        
        # Load existing or create new
        if yaml_file.exists():
            with open(yaml_file) as f:
                content = yaml.safe_load(f) or {'repos': []}
        else:
            yaml_file.parent.mkdir(parents=True, exist_ok=True)
            content = {'repos': []}
        
        # Check for duplicates
        existing = [r for r in content.get('repos', []) if r['name'] == repo_data['name']]
        if existing:
            raise ValueError(f"Repository {repo_data['name']} already exists in {yaml_file}")
        
        # Add repository
        content.setdefault('repos', []).append(repo_data)
        
        # Write back with nice formatting
        with open(yaml_file, 'w') as f:
            yaml.dump(content, f, default_flow_style=False, sort_keys=False, indent=2)
        
        return yaml_file
    
    def remove_repo(self, team: str, repo_name: str) -> bool:
        """Remove a repository from team's YAML file"""
        yaml_file = self.repo_dir / f"{team}-repos.yml"
        
        if not yaml_file.exists():
            return False
        
        with open(yaml_file) as f:
            content = yaml.safe_load(f)
        
        if not content or 'repos' not in content:
            return False
        
        # Filter out the repo
        original_length = len(content['repos'])
        content['repos'] = [r for r in content['repos'] if r['name'] != repo_name]
        
        if len(content['repos']) == original_length:
            return False
        
        # Write back
        with open(yaml_file, 'w') as f:
            yaml.dump(content, f, default_flow_style=False, sort_keys=False, indent=2)
        
        return True
    
    def update_repo(self, team: str, repo_name: str, updates: Dict) -> bool:
        """Update a repository's configuration"""
        yaml_file = self.repo_dir / f"{team}-repos.yml"
        
        if not yaml_file.exists():
            return False
        
        with open(yaml_file) as f:
            content = yaml.safe_load(f)
        
        if not content or 'repos' not in content:
            return False
        
        # Find and update repo
        found = False
        for repo in content['repos']:
            if repo['name'] == repo_name:
                repo.update(updates)
                found = True
                break
        
        if not found:
            return False
        
        # Write back
        with open(yaml_file, 'w') as f:
            yaml.dump(content, f, default_flow_style=False, sort_keys=False, indent=2)
        
        return True
    
    def get_repo(self, repo_name: str) -> Optional[Dict]:
        """Find a repository by name across all YAML files"""
        for yaml_file in self.repo_dir.glob("*.yml"):
            with open(yaml_file) as f:
                content = yaml.safe_load(f)
                if content and 'repos' in content:
                    for repo in content['repos']:
                        if repo['name'] == repo_name:
                            return {**repo, '_source': yaml_file.name}
        return None
    
    def list_repos(self, team: Optional[str] = None) -> List[Dict]:
        """List all repositories, optionally filtered by team"""
        repos = []
        
        for yaml_file in self.repo_dir.glob("*.yml"):
            with open(yaml_file) as f:
                content = yaml.safe_load(f)
                if content and 'repos' in content:
                    for repo in content['repos']:
                        if team is None or repo.get('team') == team:
                            repos.append({**repo, '_source': yaml_file.name})
        
        return repos
    
    def add_team_member(self, team: str, username: str, role: str = "member") -> bool:
        """Add a member to a team"""
        yaml_file = self.team_dir / f"{team}.yml"
        
        if not yaml_file.exists():
            return False
        
        with open(yaml_file) as f:
            content = yaml.safe_load(f)
        
        if not content or 'teams' not in content:
            return False
        
        # Find the team
        for t in content['teams']:
            if t['name'] == team:
                members = t.setdefault('members', [])
                
                # Check if member already exists
                if any(m['username'] == username for m in members):
                    return False
                
                members.append({'username': username, 'role': role})
                break
        else:
            return False
        
        # Write back
        with open(yaml_file, 'w') as f:
            yaml.dump(content, f, default_flow_style=False, sort_keys=False, indent=2)
        
        return True


def main():
    """CLI interface"""
    if len(sys.argv) < 2:
        print("Usage: yaml-helpers.py <command> [args...]", file=sys.stderr)
        print("\nCommands:", file=sys.stderr)
        print("  add-repo <team> <json>     - Add repository from JSON", file=sys.stderr)
        print("  remove-repo <team> <name>  - Remove repository", file=sys.stderr)
        print("  update-repo <team> <name> <json> - Update repository", file=sys.stderr)
        print("  get-repo <name>            - Get repository by name", file=sys.stderr)
        print("  list-repos [team]          - List repositories", file=sys.stderr)
        print("  add-member <team> <user> [role] - Add team member", file=sys.stderr)
        sys.exit(1)
    
    command = sys.argv[1]
    root = Path.cwd()
    helper = YAMLHelper(root)
    
    try:
        if command == "add-repo":
            team = sys.argv[2]
            repo_data = json.loads(sys.argv[3])
            yaml_file = helper.add_repo(team, repo_data)
            print(f"Added repository to {yaml_file}")
        
        elif command == "remove-repo":
            team = sys.argv[2]
            repo_name = sys.argv[3]
            if helper.remove_repo(team, repo_name):
                print(f"Removed {repo_name}")
            else:
                print(f"Repository {repo_name} not found", file=sys.stderr)
                sys.exit(1)
        
        elif command == "update-repo":
            team = sys.argv[2]
            repo_name = sys.argv[3]
            updates = json.loads(sys.argv[4])
            if helper.update_repo(team, repo_name, updates):
                print(f"Updated {repo_name}")
            else:
                print(f"Repository {repo_name} not found", file=sys.stderr)
                sys.exit(1)
        
        elif command == "get-repo":
            repo_name = sys.argv[2]
            repo = helper.get_repo(repo_name)
            if repo:
                print(json.dumps(repo, indent=2))
            else:
                print(f"Repository {repo_name} not found", file=sys.stderr)
                sys.exit(1)
        
        elif command == "list-repos":
            team = sys.argv[2] if len(sys.argv) > 2 else None
            repos = helper.list_repos(team)
            print(json.dumps(repos, indent=2))
        
        elif command == "add-member":
            team = sys.argv[2]
            username = sys.argv[3]
            role = sys.argv[4] if len(sys.argv) > 4 else "member"
            if helper.add_team_member(team, username, role):
                print(f"Added {username} to {team}")
            else:
                print(f"Failed to add member", file=sys.stderr)
                sys.exit(1)
        
        else:
            print(f"Unknown command: {command}", file=sys.stderr)
            sys.exit(1)
    
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
