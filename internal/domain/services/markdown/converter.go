// Package markdown provides services for converting structured content to markdown.
package markdown

import (
	"fmt"
	"strings"
)

// Node represents a hierarchical element for the conversion process.
type Node struct {
	TagName  string
	Copy     string
	Children []*Node
	Data     map[string]any
}

// Converter handles the conversion of node structures to markdown.
type Converter struct{}

// NewConverter creates a new markdown converter.
func NewConverter() *Converter {
	return &Converter{}
}

// ConvertNodesToMarkdown generates a markdown string from a slice of nodes
// by first building a tree and then traversing it.
func (c *Converter) ConvertNodesToMarkdown(nodesData []any) (string, error) {
	if len(nodesData) == 0 {
		return "", nil
	}

	nodeMap := make(map[string]*Node)
	var rootNodes []*Node

	for _, nodeInter := range nodesData {
		nodeDataMap, ok := nodeInter.(map[string]any)
		if !ok {
			continue
		}

		id, _ := nodeDataMap["id"].(string)
		tagName, _ := nodeDataMap["tagName"].(string)
		copyText, _ := nodeDataMap["copy"].(string)

		if id == "" {
			continue
		}

		node := &Node{
			TagName:  tagName,
			Copy:     copyText,
			Data:     nodeDataMap,
			Children: []*Node{},
		}
		nodeMap[id] = node
	}

	for _, nodeInter := range nodesData {
		nodeDataMap, ok := nodeInter.(map[string]any)
		if !ok {
			continue
		}

		id, _ := nodeDataMap["id"].(string)
		parentID, _ := nodeDataMap["parentId"].(string)

		node, exists := nodeMap[id]
		if !exists {
			continue
		}

		parent, parentExists := nodeMap[parentID]
		if parentExists {
			parent.Children = append(parent.Children, node)
		} else {
			rootNodes = append(rootNodes, node)
		}
	}

	var markdownBuilder strings.Builder
	for _, rootNode := range rootNodes {
		nodeType, _ := rootNode.Data["nodeType"].(string)

		// If the root is the special "Markdown" container, process its children.
		if nodeType == "Markdown" {
			for _, childNode := range rootNode.Children {
				markdownBuilder.WriteString(c.convertNode(childNode))
			}
		} else if c.isContentBlockNode(rootNode.TagName) {
			// Otherwise, handle cases where a content block might be a root itself.
			markdownBuilder.WriteString(c.convertNode(rootNode))
		}
	}

	return strings.TrimSpace(markdownBuilder.String()), nil
}

func (c *Converter) isContentBlockNode(tagName string) bool {
	switch tagName {
	case "h1", "h2", "h3", "h4", "p", "ul", "ol", "img":
		return true
	default:
		return false
	}
}

func (c *Converter) convertNode(node *Node) string {
	switch node.TagName {
	case "h1":
		return c.processHeader(node, "#")
	case "h2":
		return c.processHeader(node, "##")
	case "h3":
		return c.processHeader(node, "###")
	case "h4":
		return c.processHeader(node, "####")
	case "p":
		return c.processParagraph(node)
	case "ul":
		return c.processList(node, "-")
	case "ol":
		return c.processList(node, "1.")
	case "img":
		return c.processImage(node)
	default:
		return c.processInline(node)
	}
}

func (c *Converter) processInline(node *Node) string {
	var contentBuilder strings.Builder
	if len(node.Children) > 0 {
		for _, child := range node.Children {
			contentBuilder.WriteString(c.processInline(child))
		}
	} else {
		contentBuilder.WriteString(node.Copy)
	}

	text := contentBuilder.String()
	switch node.TagName {
	case "strong":
		return fmt.Sprintf("**%s**", text)
	case "em":
		return fmt.Sprintf("*%s*", text)
	case "code":
		return fmt.Sprintf("`%s`", text)
	default:
		return text
	}
}

func (c *Converter) processHeader(node *Node, prefix string) string {
	content := c.processInline(node)
	return fmt.Sprintf("%s %s\n\n", prefix, content)
}

func (c *Converter) processParagraph(node *Node) string {
	content := c.processInline(node)
	return fmt.Sprintf("%s\n\n", content)
}

func (c *Converter) processList(node *Node, prefix string) string {
	var listBuilder strings.Builder
	itemCount := 1
	for _, liNode := range node.Children {
		if liNode.TagName == "li" {
			var itemContentBuilder strings.Builder
			for _, contentNode := range liNode.Children {
				itemContentBuilder.WriteString(strings.TrimSpace(c.convertNode(contentNode)))
			}

			if prefix == "1." {
				listBuilder.WriteString(fmt.Sprintf("%d. %s\n", itemCount, itemContentBuilder.String()))
				itemCount++
			} else {
				listBuilder.WriteString(fmt.Sprintf("%s %s\n", prefix, itemContentBuilder.String()))
			}
		}
	}
	return listBuilder.String() + "\n"
}

func (c *Converter) processImage(node *Node) string {
	src, _ := node.Data["src"].(string)
	alt, _ := node.Data["alt"].(string)
	return fmt.Sprintf("![%s](%s)", alt, src)
}
