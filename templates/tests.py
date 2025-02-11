import unittest
from unittest.mock import patch, Mock
import re
from bs4 import BeautifulSoup
import requests

from python_final import DynatraceService

class TestDynatraceService(unittest.TestCase):
    def setUp(self):
        """Set up test cases"""
        self.valid_agent = "agent-1.0"
        self.valid_url = "https://docs.dynatrace.com/test"
        self.service = DynatraceService(self.valid_agent, self.valid_url)

    def test_init_valid_inputs(self):
        """Test initialization with valid inputs"""
        service = DynatraceService(self.valid_agent, self.valid_url)
        self.assertEqual(service.dt1AVersion, self.valid_agent)
        self.assertEqual(service.url, self.valid_url)

    def test_init_invalid_agent(self):
        """Test initialization with invalid agent version"""
        with self.assertRaises(ValueError):
            DynatraceService("", self.valid_url)
        with self.assertRaises(ValueError):
            DynatraceService("invalid_no_hyphen", self.valid_url)

    def test_init_invalid_url(self):
        """Test initialization with invalid URL"""
        invalid_urls = [
            "",
            "not_a_url",
            "http://",
            "https://"
        ]
        for url in invalid_urls:
            with self.assertRaises(ValueError):
                DynatraceService(self.valid_agent, url)

    def test_validate_url(self):
        """Test URL validation method"""
        valid_urls = [
            "https://docs.dynatrace.com",
            "http://example.com",
            "https://test.com/path?param=value"
        ]
        invalid_urls = [
            "",
            "not_a_url",
            "http://",
            "https://"
        ]

        for url in valid_urls:
            self.assertTrue(self.service._validate_url(url))

        for url in invalid_urls:
            self.assertFalse(self.service._validate_url(url))

    def test_validate_regex_pattern(self):
        """Test regex pattern validation method"""
        valid_patterns = [
            re.compile(r'NGINX\s\d|NGINX\d'),
            re.compile(r'\d+\.\d+(?:\.\d+)?'),
            r'test\d+'
        ]
        invalid_patterns = [
            r'[invalid',  # Unmatched bracket
            r'(\d+',     # Unmatched parenthesis
        ]

        for pattern in valid_patterns:
            self.assertTrue(self.service._validate_regex_pattern(pattern))

        for pattern in invalid_patterns:
            self.assertFalse(self.service._validate_regex_pattern(pattern))

    @patch('requests.get')
    def test_get_nginx_version_success(self, mock_get):
        """Test successful NGINX version extraction"""
        # Mock HTML content
        html_content = """
        <html>
            <h3 id="new-technology-support">New Technology Support</h3>
            <p>Support for NGINX 1.24.0</p>
        </html>
        """
        mock_response = Mock()
        mock_response.content = html_content.encode('utf-8')
        mock_response.status_code = 200
        mock_get.return_value = mock_response

        version = self.service.get_nginx_version()
        self.assertEqual(version, "1.24.0")

    @patch('requests.get')
    def test_get_nginx_version_no_element(self, mock_get):
        """Test when target element is not found"""
        html_content = "<html><body>No target element here</body></html>"
        mock_response = Mock()
        mock_response.content = html_content.encode('utf-8')
        mock_response.status_code = 200
        mock_get.return_value = mock_response

        version = self.service.get_nginx_version()
        self.assertIsNone(version)

    @patch('requests.get')
    def test_get_nginx_version_network_error(self, mock_get):
        """Test network error handling"""
        mock_get.side_effect = requests.exceptions.RequestException("Network error")

        with self.assertRaises(RuntimeError):
            self.service.get_nginx_version()

    @patch('requests.get')
    def test_get_nginx_version_invalid_version_format(self, mock_get):
        """Test handling of invalid version format"""
        html_content = """
        <html>
            <h3 id="new-technology-support">New Technology Support</h3>
            <p>Support for NGINX invalid.version</p>
        </html>
        """
        mock_response = Mock()
        mock_response.content = html_content.encode('utf-8')
        mock_response.status_code = 200
        mock_get.return_value = mock_response

        version = self.service.get_nginx_version()
        self.assertIsNone(version)

if __name__ == '__main__':
    unittest.main()
