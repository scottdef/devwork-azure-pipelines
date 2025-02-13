import unittest
import os
import logging
import requests
from bs4 import BeautifulSoup

from python_final import DynatraceService
from cmds import get_oneagent_nginx_version
from click.testing import CliRunner

class TestDynatraceIntegration(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        """Setup test environment variables and logging"""
        cls.test_url = "https://docs.dynatrace.com/docs/whats-new/release-notes/oneagent/sprint-297"
        cls.test_agent_version = "agent-1.297"
        
        # Setup logging for integration tests
        logging.basicConfig(
            level=logging.INFO,
            format='%(asctime)s - %(levelname)s - %(message)s'
        )
        cls.logger = logging.getLogger(__name__)

    def setUp(self):
        """Setup test instance"""
        self.service = DynatraceService(
            agent_version=self.test_agent_version,
            version_url=self.test_url
        )

    def verify_url_accessibility(self):
        """Helper method to verify if test URL is accessible"""
        try:
            response = requests.head(self.test_url, timeout=5)
            return response.status_code == 200
        except requests.RequestException:
            return False

    def test_live_url_accessibility(self):
        """Test if the Dynatrace documentation URL is accessible"""
        self.assertTrue(
            self.verify_url_accessibility(),
            "Dynatrace documentation URL is not accessible"
        )

    def test_nginx_version_format(self):
        """Test if returned NGINX version follows semantic versioning"""
        try:
            version = self.service.get_nginx_version()
            self.assertIsNotNone(version, "NGINX version should not be None")
            
            # Verify version format (major.minor.patch)
            version_parts = version.split('.')
            self.assertEqual(len(version_parts), 3, "Version should have three parts")
            
            # Verify each part is a number
            for part in version_parts:
                self.assertTrue(part.isdigit(), "Version parts should be numbers")
                
        except Exception as e:
            self.fail(f"Failed to get NGINX version: {str(e)}")

    def test_html_structure(self):
        """Test if the HTML structure contains expected elements"""
        try:
            response = requests.get(self.test_url)
            response.raise_for_status()
            soup = BeautifulSoup(response.content, 'html.parser')
            
            # Verify presence of key elements
            tech_support = soup.find("h3", id="new-technology-support")
            self.assertIsNotNone(
                tech_support,
                "New technology support section not found"
            )
            
            # Verify NGINX information exists somewhere in the document
            nginx_text = soup.find(string=lambda text: 'NGINX' in str(text))
            self.assertIsNotNone(nginx_text, "NGINX information not found")
            
        except requests.RequestException as e:
            self.fail(f"Failed to fetch HTML: {str(e)}")
        except Exception as e:
            self.fail(f"Failed to parse HTML: {str(e)}")

    def test_cli_basic_integration(self):
        """Test basic CLI command integration with default values"""
        runner = CliRunner()
        result = runner.invoke(get_oneagent_nginx_version)
        
        self.assertEqual(
            result.exit_code, 0,
            f"CLI command with defaults failed with: {result.output}"
        )
        
        # Verify output contains version number
        version_pattern = r'\d+\.\d+\.\d+'
        self.assertRegex(
            result.output,
            version_pattern,
            "CLI output should contain version number"
        )

    def test_cli_custom_version(self):
        """Test CLI with custom version parameter"""
        runner = CliRunner()
        test_cases = [
            ('1.297', 0),  # Valid version, expect success
            ('agent-1.297', 0),  # Valid version with prefix, expect success
            ('invalid', 1),  # Invalid version, expect failure
            ('', 1)  # Empty version, expect failure
        ]
        
        for version, expected_exit_code in test_cases:
            with self.subTest(version=version):
                result = runner.invoke(get_oneagent_nginx_version, ['--raw-version', version])
                self.assertEqual(
                    result.exit_code, 
                    expected_exit_code,
                    f"CLI command with version {version} failed: {result.output}"
                )

    def test_cli_custom_url(self):
        """Test CLI with custom URL parameter"""
        runner = CliRunner()
        test_cases = [
            (self.test_url, 0),  # Valid URL, expect success
            ('https://invalid.url.com', 1),  # Invalid URL, expect failure
            ('not-a-url', 1),  # Malformed URL, expect failure
        ]
        
        for url, expected_exit_code in test_cases:
            with self.subTest(url=url):
                result = runner.invoke(get_oneagent_nginx_version, [
                    '--raw-version', '1.297',
                    '--version-url', url
                ])
                self.assertEqual(
                    result.exit_code,
                    expected_exit_code,
                    f"CLI command with URL {url} failed: {result.output}"
                )

    def test_cli_combined_parameters(self):
        """Test CLI with multiple parameters combined"""
        runner = CliRunner()
        test_cases = [
            # (version, url, expected_exit_code)
            ('1.297', self.test_url, 0),  # All valid
            ('agent-1.297', self.test_url, 0),  # Valid with prefix
            ('1.297', 'invalid-url', 1),  # Invalid URL
            ('invalid', self.test_url, 1),  # Invalid version
        ]
        
        for version, url, expected_exit_code in test_cases:
            with self.subTest(version=version, url=url):
                result = runner.invoke(get_oneagent_nginx_version, [
                    '--raw-version', version,
                    '--version-url', url
                ])
                self.assertEqual(
                    result.exit_code,
                    expected_exit_code,
                    f"CLI command with version {version} and URL {url} failed: {result.output}"
                )

    def test_cli_help_output(self):
        """Test CLI help command output"""
        runner = CliRunner()
        result = runner.invoke(get_oneagent_nginx_version, ['--help'])
        
        self.assertEqual(result.exit_code, 0)
        self.assertIn('--raw-version', result.output)
        self.assertIn('--version-url', result.output)
        self.assertIn('help', result.output.lower())

    def test_error_handling_integration(self):
        """Test error handling with invalid inputs"""
        # Test with invalid URL
        with self.assertRaises(ValueError):
            DynatraceService(
                agent_version=self.test_agent_version,
                version_url="invalid-url"
            )
        
        # Test with invalid agent version
        with self.assertRaises(ValueError):
            DynatraceService(
                agent_version="invalid-version",
                version_url=self.test_url
            )

    def test_full_workflow(self):
        """Test the complete workflow from URL fetch to version extraction"""
        try:
            # 1. Verify URL accessibility
            self.assertTrue(self.verify_url_accessibility())
            
            # 2. Get NGINX version
            version = self.service.get_nginx_version()
            self.assertIsNotNone(version)
            
            # 3. Verify version format
            version_parts = version.split('.')
            self.assertEqual(len(version_parts), 3)
            
            # 4. Log success
            self.logger.info(f"Successfully retrieved NGINX version: {version}")
            
        except Exception as e:
            self.fail(f"Full workflow test failed: {str(e)}")

    def tearDown(self):
        """Cleanup after tests"""
        self.service = None

if __name__ == '__main__':
    unittest.main(verbosity=2)
