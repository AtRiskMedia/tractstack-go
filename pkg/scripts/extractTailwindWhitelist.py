#!/usr/bin/env python3

import json
import sys
from pathlib import Path
from bs4 import BeautifulSoup
import re
from typing import Set


def extract_classes_from_html(html_content: str) -> Set[str]:
    """Extract classes from HTML using BeautifulSoup"""
    classes = set()
    try:
        soup = BeautifulSoup(html_content, "html.parser")
        for element in soup.find_all(attrs={"class": True}):
            class_attr = element.get("class")
            if isinstance(class_attr, list):
                classes.update(class_attr)
            elif isinstance(class_attr, str):
                classes.update(class_attr.split())
    except Exception as e:
        print(f"Error parsing HTML: {e}")
    return classes


def extract_classes_from_code(code_content: str) -> Set[str]:
    """Extract classes from JavaScript/Go code using multiple methods"""
    classes = set()

    # Method 1: Quote pattern extraction
    quote_patterns = [r'"([^"]*)"', r"'([^']*)'", r"`([^`]*)`"]

    for pattern in quote_patterns:
        matches = re.findall(pattern, code_content)
        for match in matches:
            tokens = match.split()
            for token in tokens:
                # Strip all illegal characters from start and end
                clean_token = re.sub(r"^[^a-zA-Z-]+|[^a-zA-Z0-9_/:.,-]+$", "", token)
                if (
                    clean_token
                    and re.match(r"^-?[a-z][a-zA-Z0-9_/:.,-]*$", clean_token)
                    and len(clean_token) < 50
                ):
                    classes.add(clean_token)

    # Method 2: Direct class attribute search
    start_pos = 0
    while True:
        class_start = code_content.find('class="', start_pos)
        if class_start == -1:
            break

        content_start = class_start + 7
        quote_end = code_content.find('"', content_start)

        if quote_end != -1:
            class_content = code_content[content_start:quote_end]
            tokens = class_content.split()
            for token in tokens:
                # Strip all illegal characters from start and end
                clean_token = re.sub(r"^[^a-zA-Z-]+|[^a-zA-Z0-9_/:.,-]+$", "", token)
                if (
                    clean_token
                    and re.match(r"^-?[a-z][a-zA-Z0-9_/:.,-]*$", clean_token)
                    and len(clean_token) < 50
                ):
                    classes.add(clean_token)

        start_pos = class_start + 1

    return classes


def scan_dist_directory(dist_path: Path) -> Set[str]:
    """Scan the built dist directory for all CSS classes"""
    all_classes = set()

    if not dist_path.exists():
        print(f"Error: dist directory not found at {dist_path}")
        return all_classes

    # Scan HTML files
    for html_file in dist_path.rglob("*.html"):
        try:
            with open(html_file, "r", encoding="utf-8") as f:
                content = f.read()
                classes = extract_classes_from_html(content)
                all_classes.update(classes)
        except Exception as e:
            print(f"Error reading {html_file}: {e}")

    # Scan JavaScript files
    for js_file in dist_path.rglob("*.js"):
        try:
            with open(js_file, "r", encoding="utf-8") as f:
                content = f.read()
                classes = extract_classes_from_code(content)
                all_classes.update(classes)
        except Exception as e:
            print(f"Error reading {js_file}: {e}")

    # Scan .mjs files (ES modules)
    for mjs_file in dist_path.rglob("*.mjs"):
        try:
            with open(mjs_file, "r", encoding="utf-8") as f:
                content = f.read()
                classes = extract_classes_from_code(content)
                all_classes.update(classes)
        except Exception as e:
            print(f"Error reading {mjs_file}: {e}")

    return all_classes


def scan_go_templates(go_templates_path: Path) -> Set[str]:
    """Scan Go template files for CSS classes"""
    all_classes = set()

    if not go_templates_path.exists():
        print(f"Warning: Go templates directory not found at {go_templates_path}")
        return all_classes

    # Scan all .go files recursively
    for go_file in go_templates_path.rglob("*.go"):
        try:
            with open(go_file, "r", encoding="utf-8") as f:
                content = f.read()
                classes = extract_classes_from_code(content)
                all_classes.update(classes)
        except Exception as e:
            print(f"Error reading {go_file}: {e}")

    return all_classes


def main():
    if len(sys.argv) != 3 and len(sys.argv) != 4:
        print(
            "Usage: python3 extractTailwindWhitelist.py <dist_path> <output_json> [go_templates_path]"
        )
        sys.exit(1)

    dist_path = Path(sys.argv[1])
    output_path = Path(sys.argv[2])
    go_templates_path = Path(sys.argv[3]) if len(sys.argv) == 4 else None

    print(f"Scanning dist directory: {dist_path}")
    dist_classes = scan_dist_directory(dist_path)
    print(f"Found {len(dist_classes)} classes in dist directory")

    go_classes = set()
    if go_templates_path:
        print(f"Scanning Go templates directory: {go_templates_path}")
        go_classes = scan_go_templates(go_templates_path)
        print(f"Found {len(go_classes)} classes in Go templates")

    # Combine all classes
    all_classes = dist_classes | go_classes

    # Minimal filtering - only exclude obvious non-CSS junk
    tailwind_classes = set()
    for cls in all_classes:
        if (
            cls
            and len(cls) > 1
            and len(cls) < 50
            and not cls.startswith("_")
            and not cls.startswith("http")
            and not cls.startswith("javascript:")
            and not cls.startswith("data:")
            and not cls.startswith("mailto:")
            and not cls.startswith("tel:")
            and not cls.isdigit()
            and cls
            not in [
                "function",
                "return",
                "const",
                "let",
                "var",
                "import",
                "export",
                "true",
                "false",
                "null",
                "undefined",
            ]
        ):
            tailwind_classes.add(cls)

    # Sort for consistent output
    sorted_classes = sorted(tailwind_classes)

    # Create output structure
    output_data = {"safelist": sorted_classes}

    # Ensure output directory exists
    output_path.parent.mkdir(parents=True, exist_ok=True)

    # Write JSON
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(output_data, f, indent=2)

    print(f"Extracted {len(sorted_classes)} total classes to {output_path}")


if __name__ == "__main__":
    main()
