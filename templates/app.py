import sys
import re
import requests
from bs4 import BeautifulSoup

# regex patterns
#pattern = re.compile('NGINX')
pattern = re.compile("NGINX\s\d|NGINX\d")

# Initialize agent_version
agent_version = 1.293
version_suffix = str(agent_version).split('.')[1]

# Create complete URL by concatenating incomplete_url and version_suffix
incomplete_url = "https://docs.dynatrace.com/docs/whats-new/release-notes/oneagent/sprint-"
complete_url = f"{incomplete_url}{version_suffix}"

try:

    resp = requests.get(complete_url)
    resp.raise_for_status()  # Raise an exception for bad status codes
    page = resp.content

except requests.exceptions.RequestException as e:
    print(f"Error retrieving the webpage: {e}")

try:

    soup = BeautifulSoup(page, 'html.parser')
    data = soup.find("h3", id="new-technology-support")
    end_data = soup.find("h3", id="end-of-support")

    if data:

        # 4. Find next element matching pattern
        tgt = data.find_all_next(string=pattern)[0]
        print(type(tgt))

        # 5. Check type of tgt and raise TypeError if incorrect
        if type(tgt) == type(soup.string):
            print("type is correct")
        else:
            actual_type = type(tgt)
            raise TypeError(f"Expected type bs4.element.NavigableString --> actual {actual_type}")
    else:
        print("Could not find element with id 'new-technology-support'")

except TypeError as e:
    print(f"Type Error: {e}")
except Exception as e:
    print(f"An error occurred: {e}")



# 1. Create the test string
#teststr = 'NGINX 1.26.1, 1.27 (NGINX module)'

# 2. Extract version numbers using regex
# This pattern matches both major.minor and major.minor.patch formats
version_pattern = r'\d+\.\d+(?:\.\d+)?'
extracted_versions = re.findall(version_pattern, tgt)

# 3. Convert versions to major.minor.patch format
formatted_versions = []
for version in extracted_versions:
    # Check if version is in major.minor format
    if version.count('.') == 1:
        version = f"{version}.0"
    formatted_versions.append(version)

max_NGINX_version = formatted_versions[-1]

# Print results
print("Building URL...")
print(f"Agent Version: {agent_version}")
print(f"Version Suffix: {version_suffix}")
print(f"Incomplete URL: {incomplete_url}")
print(f"Complete URL: {complete_url}")

print("Building version list...")
print(f"h3 new-technology-support: {data}")
print(f"version list element-----: {tgt}")
print(type(tgt))
#print(f"version regex element-----: {tgt_exists}")


print(f"Prepping Max Version...")
print(f"Original string: {tgt}")
print(f"Extracted versions: {extracted_versions}")
print(f"Formatted versions: {formatted_versions}")
print(f"max_NGINX_version: {max_NGINX_version}")
