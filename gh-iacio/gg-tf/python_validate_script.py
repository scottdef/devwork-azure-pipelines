#!/usr/bin/env python3
"""
validate-yaml.py - Advanced YAML validation for GitHub governance
Written in the spirit of Rob Pike and Ken Thompson: simple, clear, fast
"""

import sys
import yaml
from pathlib import Path
from typing import Dict, List, Set, Tuple
from dataclasses import dataclass


@dataclass
class ValidationResult:
    """Validation result. Simple."""
    success: bool
    errors: List[str]
    warnings: List[str]


class GovernanceValidator:
    """Validates GitHub governance configuration"""
    
    def __init__(self, root_path: Path):
        self.root = root_path
        self.repos: Dict[str, dict] = {}
        self.teams: Dict[str, dict] = {}
        self.errors: List[str] = []
        self.warnings: List[str] = []
    
    def validate_all(self) -> ValidationResult:
        """Run all validations"""
        self._load_yamls()
        self._validate_repos()
        self._validate_teams()
        self._validate_references()
        self._validate_duplicates()
        
        return ValidationResult(
            success=len(self.errors) == 0,
            errors=self.errors,
            warnings=self.warnings
        )
    
    def _load_yamls(self):
        """Load all YAML files"""
        # Load repos
        repo_dir = self.root / "repo-yamls"
        if repo_dir.exists():
            for yaml_file in repo_dir.glob("*.yml"):
                try:
                    with open(yaml_file) as f:
                        data = yaml.safe_load(f)
                        if data and "repos" in data:
                            for repo in data["repos"]:
                                self.repos[repo["name"]] = {
                                    **repo,
                                    "_source": yaml_file.name
                                }
                except Exception as e:
                    self.errors.append(f"{yaml_file.name}: Failed to load: {e}")
        
        # Load teams
        team_dir = self.root / "team-yamls"
        if team_dir.exists():
            for yaml_file in team_dir.glob("*.yml"):
                try:
                    with open(yaml_file) as f:
                        data = yaml.safe_load(f)
                        if data and "teams" in data:
                            for team in data["teams"]:
                                self.teams[team["name"]] = {
                                    **team,
                                    "_source": yaml_file.name
                                }
                except Exception as e:
                    self.errors.append(f"{yaml_file.name}: Failed to load: {e}")
    
    def _validate_repos(self):
        """Validate repository definitions"""
        required_fields = ["name"]
        valid_visibilities = ["public", "private", "internal"]
        valid_permissions = ["pull", "triage", "push", "maintain", "admin"]
        
        for name, repo in self.repos.items():
            source = repo.get("_source", "unknown")
            
            # Check required fields
            for field in required_fields:
                if field not in repo or not repo[field]:
                    self.errors.append(f"{source}: Repo missing required field '{field}'")
            
            # Validate visibility
            if "visibility" in repo and repo["visibility"] not in valid_visibilities:
                self.errors.append(
                    f"{source}: Repo '{name}' has invalid visibility '{repo['visibility']}'"
                )
            
            # Validate permission
            if "permission" in repo and repo["permission"] not in valid_permissions:
                self.errors.append(
                    f"{source}: Repo '{name}' has invalid permission '{repo['permission']}'"
                )
            
            # Check repo name format (GitHub restrictions)
            if not name.replace("-", "").replace("_", "").replace(".", "").isalnum():
                self.warnings.append(
                    f"{source}: Repo '{name}' has special characters that may not be allowed"
                )
    
    def _validate_teams(self):
        """Validate team definitions"""
        required_fields = ["name"]
        valid_privacies = ["secret", "closed"]
        valid_roles = ["member", "maintainer"]
        
        for name, team in self.teams.items():
            source = team.get("_source", "unknown")
            
            # Check required fields
            for field in required_fields:
                if field not in team or not team[field]:
                    self.errors.append(f"{source}: Team missing required field '{field}'")
            
            # Validate privacy
            if "privacy" in team and team["privacy"] not in valid_privacies:
                self.errors.append(
                    f"{source}: Team '{name}' has invalid privacy '{team['privacy']}'"
                )
            
            # Validate members
            if "members" in team:
                seen_usernames = set()
                for member in team["members"]:
                    if "username" not in member:
                        self.errors.append(
                            f"{source}: Team '{name}' has member without username"
                        )
                        continue
                    
                    username = member["username"]
                    if username in seen_usernames:
                        self.errors.append(
                            f"{source}: Team '{name}' has duplicate member '{username}'"
                        )
                    seen_usernames.add(username)
                    
                    if "role" in member and member["role"] not in valid_roles:
                        self.errors.append(
                            f"{source}: Team '{name}' member '{username}' has invalid role"
                        )
    
    def _validate_references(self):
        """Validate that references between resources exist"""
        for name, repo in self.repos.items():
            source = repo.get("_source", "unknown")
            
            # Check team references
            if "team" in repo:
                team_name = repo["team"]
                if team_name not in self.teams:
                    self.errors.append(
                        f"{source}: Repo '{name}' references non-existent team '{team_name}'"
                    )
        
        for name, team in self.teams.items():
            source = team.get("_source", "unknown")
            
            # Check parent team references
            if "parent_name" in team:
                parent = team["parent_name"]
                if parent not in self.teams:
                    self.errors.append(
                        f"{source}: Team '{name}' references non-existent parent '{parent}'"
                    )
    
    def _validate_duplicates(self):
        """Check for duplicate resources"""
        # Already handled by dictionary keys, but check across sources
        repo_sources: Dict[str, List[str]] = {}
        for name, repo in self.repos.items():
            source = repo.get("_source", "unknown")
            if name not in repo_sources:
                repo_sources[name] = []
            repo_sources[name].append(source)
        
        for name, sources in repo_sources.items():
            if len(sources) > 1:
                self.errors.append(
                    f"Repo '{name}' defined in multiple files: {', '.join(sources)}"
                )
        
        team_sources: Dict[str, List[str]] = {}
        for name, team in self.teams.items():
            source = team.get("_source", "unknown")
            if name not in team_sources:
                team_sources[name] = []
            team_sources[name].append(source)
        
        for name, sources in team_sources.items():
            if len(sources) > 1:
                self.errors.append(
                    f"Team '{name}' defined in multiple files: {', '.join(sources)}"
                )


def main():
    """Main entry point"""
    if len(sys.argv) > 1:
        root = Path(sys.argv[1])
    else:
        root = Path.cwd()
    
    if not root.exists():
        print(f"Error: Path {root} does not exist", file=sys.stderr)
        sys.exit(1)
    
    validator = GovernanceValidator(root)
    result = validator.validate_all()
    
    # Print results
    if result.warnings:
        print("\nWarnings:")
        for warning in result.warnings:
            print(f"  ⚠ {warning}")
    
    if result.errors:
        print("\nErrors:")
        for error in result.errors:
            print(f"  ✗ {error}")
        print(f"\nValidation failed with {len(result.errors)} error(s)")
        sys.exit(1)
    else:
        print("\n✓ All validations passed")
        if result.warnings:
            print(f"  ({len(result.warnings)} warning(s))")
        sys.exit(0)


if __name__ == "__main__":
    main()
