# standard modules
import sys
import re
import logging
from urllib.parse import urlparse

# external-modules
import requests
from bs4 import BeautifulSoup

# project imports

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

class DynatraceService(object):
    def __init__(
        self,
        agent_version: str,
        version_url: str
    ):
        if not agent_version or not isinstance(agent_version, str):
            raise ValueError("Agent version must be a non-empty string")

        if not self._validate_url(version_url):
            raise ValueError("Invalid URL format")

        self.dt1AVersion = agent_version
        try:
            version_suffix = agent_version.split('.')[1]
        except IndexError:
            raise ValueError("Agent version must contain a dot")

        self.url = version_url

    @staticmethod
    def _validate_url(url: str) -> bool:
        """Validate URL format."""
        try:
            result = urlparse(url)
            return all([result.scheme, result.netloc])
        except Exception:
            return False

    def _validate_regex_pattern(self, pattern: re.Pattern) -> bool:
        """Validate regex pattern compilation."""
        try:
            if not isinstance(pattern, re.Pattern):
                re.compile(pattern)
            return True
        except re.error:
            return False

    def get_nginx_version(self):
        # regex patterns
        tech_pattern = re.compile(r'NGINX\s\d|NGINX\d')
        version_pattern = re.compile(r'\d+\.\d+(?:\.\d+)?')

        # Validate regex patterns
        if not all(self._validate_regex_pattern(pattern) for pattern in [tech_pattern, version_pattern]):
            raise ValueError("Invalid regex pattern")

        try:
            resp = requests.get(self.url, timeout=30)
            resp.raise_for_status()  # Raise exception for bad status codes
            page = resp.content

        except requests.exceptions.RequestException as e:
            logger.error(f"Error retrieving NGINX version: {str(e)}")
            raise RuntimeError(f"Encountered an issue while trying to retrieve NGINX version: {str(e)}")

        try:
            soup = BeautifulSoup(page, 'html.parser')

            # find if new NGINX versions are supported then extract the versions
            data = soup.find("h3", id="new-technology-support")
            if not data:
                logger.warning("Could not find element with id 'new-technology-support'")
                return None

            nginx_matches = data.find_all_next(string=tech_pattern)
            if not nginx_matches:
                logger.warning("No NGINX version information found")
                return None

            tgt = nginx_matches[0]

            # pattern matches both major.minor and major.minor.patch formats
            extracted_versions = version_pattern.findall(tgt)
            if not extracted_versions:
                logger.warning("No version numbers found in NGINX information")
                return None

            # convert versions to major.minor.patch format
            formatted_versions = []
            for version in extracted_versions:
                if version.count('.') == 1:
                    version = f"{version}.0"
                formatted_versions.append(version)

            NGINX_version = formatted_versions[-1]
            return NGINX_version

        except Exception as e:
            logger.error(f"Error parsing NGINX version: {str(e)}")
            raise RuntimeError(f"Error parsing NGINX version: {str(e)}")
