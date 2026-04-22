#!/usr/bin/env python3

import json
import sys
from pathlib import Path
from bs4 import BeautifulSoup
import re
from typing import List, Optional, Set, Tuple


JS_KEYWORDS = {
    "function",
    "return",
    "const",
    "let",
    "var",
    "import",
    "export",
    "default",
    "true",
    "false",
    "null",
    "undefined",
    "if",
    "else",
    "for",
    "while",
    "switch",
    "case",
    "break",
    "continue",
    "new",
    "class",
    "try",
    "catch",
    "finally",
    "throw",
}

STANDALONE_UTILITIES = {
    "block",
    "inline",
    "inline-block",
    "inline-flex",
    "inline-grid",
    "flex",
    "grid",
    "hidden",
    "contents",
    "table",
    "flow-root",
    "static",
    "fixed",
    "absolute",
    "relative",
    "sticky",
    "italic",
    "not-italic",
    "underline",
    "overline",
    "line-through",
    "no-underline",
    "uppercase",
    "lowercase",
    "capitalize",
    "normal-case",
    "truncate",
    "antialiased",
    "subpixel-antialiased",
    "sr-only",
    "not-sr-only",
    "visible",
    "invisible",
    "isolate",
    "isolation-auto",
    "transform",
    "transform-gpu",
    "transform-none",
    "filter",
    "filter-none",
    "backdrop-filter",
    "backdrop-filter-none",
    "appearance-none",
}

UTILITY_PREFIXES = {
    "m",
    "mx",
    "my",
    "mt",
    "mr",
    "mb",
    "ml",
    "ms",
    "me",
    "p",
    "px",
    "py",
    "pt",
    "pr",
    "pb",
    "pl",
    "ps",
    "pe",
    "w",
    "h",
    "min-w",
    "max-w",
    "min-h",
    "max-h",
    "size",
    "inset",
    "inset-x",
    "inset-y",
    "top",
    "right",
    "bottom",
    "left",
    "z",
    "order",
    "col",
    "row",
    "grid",
    "grid-cols",
    "grid-rows",
    "grid-flow",
    "auto-cols",
    "auto-rows",
    "gap",
    "gap-x",
    "gap-y",
    "space-x",
    "space-y",
    "flex",
    "basis",
    "grow",
    "shrink",
    "items",
    "justify",
    "content",
    "place-content",
    "place-items",
    "place-self",
    "self",
    "object",
    "overflow",
    "overflow-x",
    "overflow-y",
    "text",
    "font",
    "leading",
    "tracking",
    "line-clamp",
    "list",
    "whitespace",
    "break",
    "decoration",
    "underline-offset",
    "placeholder",
    "align",
    "vertical-align",
    "bg",
    "from",
    "via",
    "to",
    "fill",
    "stroke",
    "border",
    "border-x",
    "border-y",
    "border-t",
    "border-r",
    "border-b",
    "border-l",
    "rounded",
    "rounded-t",
    "rounded-r",
    "rounded-b",
    "rounded-l",
    "rounded-tl",
    "rounded-tr",
    "rounded-br",
    "rounded-bl",
    "outline",
    "ring",
    "ring-offset",
    "shadow",
    "opacity",
    "mix-blend",
    "bg-blend",
    "blur",
    "brightness",
    "contrast",
    "drop-shadow",
    "grayscale",
    "hue-rotate",
    "invert",
    "saturate",
    "sepia",
    "backdrop",
    "backdrop-blur",
    "backdrop-brightness",
    "backdrop-contrast",
    "backdrop-grayscale",
    "backdrop-hue-rotate",
    "backdrop-invert",
    "backdrop-opacity",
    "backdrop-saturate",
    "backdrop-sepia",
    "transition",
    "duration",
    "ease",
    "delay",
    "animate",
    "scale",
    "rotate",
    "translate",
    "skew",
    "origin",
    "cursor",
    "caret",
    "accent",
    "appearance",
    "pointer-events",
    "resize",
    "scroll",
    "scroll-m",
    "scroll-mx",
    "scroll-my",
    "scroll-mt",
    "scroll-mr",
    "scroll-mb",
    "scroll-ml",
    "scroll-p",
    "scroll-px",
    "scroll-py",
    "scroll-pt",
    "scroll-pr",
    "scroll-pb",
    "scroll-pl",
    "snap",
    "touch",
    "select",
    "will-change",
    "aspect",
    "container",
    "prose",
    "divide",
    "divide-x",
    "divide-y",
}

VARIANT_PREFIXES = {
    "xs",
    "sm",
    "md",
    "lg",
    "xl",
    "2xl",
    "hover",
    "focus",
    "focus-within",
    "focus-visible",
    "active",
    "visited",
    "disabled",
    "checked",
    "required",
    "invalid",
    "first",
    "last",
    "odd",
    "even",
    "empty",
    "read-only",
    "group-hover",
    "group-focus",
    "peer-hover",
    "peer-focus",
    "motion-safe",
    "motion-reduce",
    "dark",
    "print",
    "rtl",
    "ltr",
    "before",
    "after",
    "placeholder",
    "selection",
}

def extract_classes_from_html(html_content: str) -> Set[str]:
    """Extract classes from HTML using BeautifulSoup"""
    classes = set()
    try:
        soup = BeautifulSoup(html_content, "html.parser")
        for element in soup.find_all(attrs={"class": True}):
            class_attr = element.get("class")
            if isinstance(class_attr, list):
                for token in class_attr:
                    if token:
                        classes.add(token.strip())
            elif isinstance(class_attr, str):
                classes.update(split_class_string(class_attr))
    except Exception as e:
        print(f"Error parsing HTML: {e}")
    return classes


def split_class_string(value: str) -> List[str]:
    return [token.strip() for token in re.split(r"\s+", value.strip()) if token.strip()]


def trim_token(token: str) -> str:
    return re.sub(r"^[^a-zA-Z0-9_\-\[\]!]+|[^a-zA-Z0-9_/:.,\-\[\]()%!]+$", "", token)


def has_balanced_brackets(token: str) -> bool:
    depth = 0
    for ch in token:
        if ch == "[":
            depth += 1
        elif ch == "]":
            depth -= 1
            if depth < 0:
                return False
    return depth == 0


def split_variants(token: str) -> List[str]:
    parts: List[str] = []
    start = 0
    bracket_depth = 0
    for idx, ch in enumerate(token):
        if ch == "[":
            bracket_depth += 1
        elif ch == "]":
            bracket_depth = max(0, bracket_depth - 1)
        elif ch == ":" and bracket_depth == 0:
            parts.append(token[start:idx])
            start = idx + 1
    parts.append(token[start:])
    return [p for p in parts if p]


def is_valid_variant(segment: str) -> bool:
    if not segment:
        return False
    if segment in VARIANT_PREFIXES:
        return True
    if segment.startswith(("group-", "peer-", "aria-", "data-", "supports-")):
        return True
    if segment.endswith("-only") or segment.endswith("-of-type"):
        return True
    if re.match(r"^[a-z0-9-]+(\[[^\]]+\])?$", segment):
        return True
    return False


def is_obvious_noise(token: str) -> bool:
    lowered = token.lower()
    if token.startswith("_"):
        return True
    if lowered in JS_KEYWORDS:
        return True
    if lowered.startswith(("http://", "https://", "javascript:", "data:", "mailto:", "tel:")):
        return True
    if lowered.startswith(("./", "../", "/home/", "/usr/", "/tmp/")):
        return True
    if lowered.endswith((".js", ".mjs", ".ts", ".tsx", ".go", ".json", ".svg", ".png", ".jpg", ".css")):
        return True
    if token.count("{") > 0 or token.count("}") > 0:
        return True
    if token.isdigit():
        return True
    return False


def is_valid_base_utility(base: str) -> bool:
    if not base:
        return False
    if base in STANDALONE_UTILITIES:
        return True
    if base.startswith("[") and base.endswith("]"):
        inner = base[1:-1]
        return bool(inner and ":" in inner and re.match(r"^[a-z0-9_\-()[\].,%#'\"/& ]+$", inner))

    working = base
    if working.startswith("!"):
        working = working[1:]
    if working.startswith("-"):
        working = working[1:]
    if not working:
        return False

    opacity_part = None
    if "/" in working and not working.endswith("/"):
        working, opacity_part = working.rsplit("/", 1)
        if not re.match(r"^\[[^\]]+\]$|^\d{1,3}$", opacity_part):
            return False

    if "[" in working:
        prefix = working.split("-[", 1)[0]
        if prefix in UTILITY_PREFIXES:
            return True
        return bool(re.match(r"^[a-z][a-z0-9-]*-\[[^\]]+\]$", working))

    if working in STANDALONE_UTILITIES:
        return True

    if working in {"not-prose", "sr-only", "not-sr-only"}:
        return True

    if "-" not in working:
        return False

    for prefix in sorted(UTILITY_PREFIXES, key=len, reverse=True):
        if working == prefix:
            return True
        if working.startswith(prefix + "-"):
            return True

    return False


def is_valid_tailwind_token(token: str) -> bool:
    if not token:
        return False
    if len(token) < 2 or len(token) > 160:
        return False
    if token.endswith(":"):
        return False
    if is_obvious_noise(token):
        return False
    if not has_balanced_brackets(token):
        return False
    if ".." in token:
        return False
    if re.search(r"[<>{};]", token):
        return False
    if "'" in token or '"' in token or "`" in token:
        return False

    parts = split_variants(token)
    if not parts:
        return False

    if len(parts) > 1:
        for variant in parts[:-1]:
            if not is_valid_variant(variant):
                return False

    return is_valid_base_utility(parts[-1])


def extract_from_class_attributes(code_content: str) -> Set[str]:
    classes: Set[str] = set()
    pattern = re.compile(r"\b(?:class|className)\s*=\s*([\"'])(.*?)\1", re.DOTALL)
    for match in pattern.finditer(code_content):
        classes.update(split_class_string(match.group(2)))
    return classes


def parse_string_literal(text: str, idx: int) -> Tuple[str, int]:
    quote = text[idx]
    idx += 1
    out: List[str] = []
    while idx < len(text):
        ch = text[idx]
        if ch == "\\" and idx + 1 < len(text):
            out.append(text[idx + 1])
            idx += 2
            continue
        if ch == quote:
            return "".join(out), idx + 1
        out.append(ch)
        idx += 1
    return "".join(out), idx


def parse_template_literal(text: str, idx: int) -> Tuple[str, int]:
    # idx points at opening backtick.
    idx += 1
    out: List[str] = []
    while idx < len(text):
        ch = text[idx]
        if ch == "\\" and idx + 1 < len(text):
            out.append(text[idx + 1])
            idx += 2
            continue
        if ch == "`":
            return "".join(out), idx + 1
        if ch == "$" and idx + 1 < len(text) and text[idx + 1] == "{":
            out.append(" ")
            idx = skip_js_expression(text, idx + 2)
            continue
        out.append(ch)
        idx += 1
    return "".join(out), idx


def skip_js_expression(text: str, idx: int) -> int:
    depth = 1
    while idx < len(text) and depth > 0:
        ch = text[idx]
        if ch in ("'", '"'):
            _, idx = parse_string_literal(text, idx)
            continue
        if ch == "`":
            _, idx = parse_template_literal(text, idx)
            continue
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
        idx += 1
    return idx


def read_argument(text: str, idx: int) -> Tuple[str, int]:
    start = idx
    depth_paren = 0
    depth_bracket = 0
    depth_brace = 0
    while idx < len(text):
        ch = text[idx]
        if ch in ("'", '"'):
            _, idx = parse_string_literal(text, idx)
            continue
        if ch == "`":
            _, idx = parse_template_literal(text, idx)
            continue
        if ch == "(":
            depth_paren += 1
        elif ch == ")":
            if depth_paren == 0 and depth_bracket == 0 and depth_brace == 0:
                break
            depth_paren = max(0, depth_paren - 1)
        elif ch == "[":
            depth_bracket += 1
        elif ch == "]":
            depth_bracket = max(0, depth_bracket - 1)
        elif ch == "{":
            depth_brace += 1
        elif ch == "}":
            depth_brace = max(0, depth_brace - 1)
        elif ch == "," and depth_paren == 0 and depth_bracket == 0 and depth_brace == 0:
            break
        idx += 1
    return text[start:idx].strip(), idx


def read_call_args(text: str, open_paren_idx: int) -> Tuple[List[str], int]:
    # open_paren_idx points at "("
    args: List[str] = []
    idx = open_paren_idx + 1
    while idx < len(text):
        while idx < len(text) and text[idx].isspace():
            idx += 1
        if idx >= len(text):
            break
        if text[idx] == ")":
            return args, idx + 1

        arg, idx = read_argument(text, idx)
        args.append(arg)
        while idx < len(text) and text[idx].isspace():
            idx += 1
        if idx < len(text) and text[idx] == ",":
            idx += 1
            continue
        if idx < len(text) and text[idx] == ")":
            return args, idx + 1
    return args, idx


def decode_literal_if_string_or_template(expr: str) -> Optional[str]:
    expr = expr.strip()
    if len(expr) < 2:
        return None
    if expr[0] == expr[-1] and expr[0] in ("'", '"'):
        return expr[1:-1]
    if expr[0] == "`" and expr[-1] == "`":
        raw = expr[1:-1]
        # Replace ${...} with a separator so static class names still split correctly.
        raw = re.sub(r"\$\{[^}]*\}", " ", raw)
        return raw
    return None


def extract_from_add_attribute_calls(code_content: str) -> Set[str]:
    classes: Set[str] = set()
    needle = "addAttribute("
    idx = 0
    while True:
        start = code_content.find(needle, idx)
        if start == -1:
            break
        args, end_idx = read_call_args(code_content, start + len("addAttribute"))
        idx = max(end_idx, start + 1)
        if len(args) < 2:
            continue

        attr_name = decode_literal_if_string_or_template(args[1])
        if attr_name not in {"class", "className"}:
            continue
        first_value = decode_literal_if_string_or_template(args[0])
        if not first_value:
            continue
        classes.update(split_class_string(first_value))
    return classes


def extract_from_broad_strings(code_content: str) -> Set[str]:
    classes: Set[str] = set()
    quote_patterns = [r'"([^"\n]*)"', r"'([^'\n]*)'", r"`([^`]*)`"]
    for pattern in quote_patterns:
        for match in re.findall(pattern, code_content):
            for raw_token in re.split(r"\s+", match):
                token = trim_token(raw_token)
                if token:
                    classes.add(token)
    return classes


def extract_classes_from_code(code_content: str) -> Set[str]:
    """Extract classes from JavaScript/TypeScript/Go source and compiled code."""
    all_candidates: Set[str] = set()

    # Tier 1: high-confidence sinks.
    all_candidates.update(extract_from_class_attributes(code_content))
    all_candidates.update(extract_from_add_attribute_calls(code_content))

    # Tier 2: broad net, validated later.
    all_candidates.update(extract_from_broad_strings(code_content))

    classes: Set[str] = set()
    for token in all_candidates:
        clean = trim_token(token)
        if is_valid_tailwind_token(clean):
            classes.add(clean)
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
        print("Usage: python3 extractTailwindWhitelist.py <dist_path> <output_json> [go_templates_path]")
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

    # Final validation pass across union.
    tailwind_classes = set()
    for cls in all_classes:
        if is_valid_tailwind_token(cls):
            tailwind_classes.add(cls)

    # Sort for consistent output
    sorted_classes = sorted(tailwind_classes)

    # Create output structure
    output_data = {
        "safelist": sorted_classes
    }

    # Ensure output directory exists
    output_path.parent.mkdir(parents=True, exist_ok=True)

    # Write JSON
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(output_data, f, indent=2)

    print(f"Extracted {len(sorted_classes)} total classes to {output_path}")

if __name__ == "__main__":
    main()
