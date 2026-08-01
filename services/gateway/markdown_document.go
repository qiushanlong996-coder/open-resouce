package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Markdown 正文到文档结构的转换。
//
// 阅读页需要三样东西：渲染用的 Markdown 原文、页内导航用的标题大纲，
// 以及选区评论锚定用的稳定内容块。作者发布的项目只持久化 Markdown，
// 因此这里在读取时解析出大纲和块。
//
// 稳定块 ID 采用「块内纯文本的哈希」：只要该块文字没有被改动，
// 跨请求、跨部署都会得到同一个 ID，已有评论仍能锚定到原位置。
// 代价是该块自身被编辑后 ID 会变化，对应评论会失去锚点；
// 要做到编辑后仍然保留锚点，需要为每个块持久化独立 ID，属于后续能力。

const maxParsedBlocks = 400

// stableTextID 由前缀和文本内容生成稳定标识。
func stableTextID(prefix, text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return prefix + "-" + hex.EncodeToString(sum[:])[:16]
}

type parsedMarkdown struct {
	Title   string
	Outline []documentOutlineItem
	Blocks  []documentBlock
}

// parseMarkdownDocument 从 Markdown 中提取标题、大纲和稳定内容块。
// 解析遵循 CommonMark 中阅读页真正用到的子集：ATX 标题、围栏代码块和段落。
func parseMarkdownDocument(markdown string) parsedMarkdown {
	result := parsedMarkdown{
		Outline: make([]documentOutlineItem, 0, 8),
		Blocks:  make([]documentBlock, 0, 16),
	}
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")

	var paragraph []string
	var code []string
	inCode := false

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		text := strings.TrimSpace(strings.Join(paragraph, " "))
		paragraph = paragraph[:0]
		if text == "" || len(result.Blocks) >= maxParsedBlocks {
			return
		}
		result.Blocks = append(result.Blocks, documentBlock{
			ID: stableTextID("block", text), Type: "paragraph", Text: text,
		})
	}
	flushCode := func() {
		text := strings.TrimRight(strings.Join(code, "\n"), "\n")
		code = code[:0]
		if strings.TrimSpace(text) == "" || len(result.Blocks) >= maxParsedBlocks {
			return
		}
		result.Blocks = append(result.Blocks, documentBlock{
			ID: stableTextID("block", text), Type: "code", Text: text,
		})
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)

		// 围栏代码块内的内容原样收集，不参与标题和段落判断。
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if inCode {
				flushCode()
				inCode = false
			} else {
				flushParagraph()
				inCode = true
			}
			continue
		}
		if inCode {
			code = append(code, line)
			continue
		}

		if level, title, ok := parseATXHeading(trimmed); ok {
			flushParagraph()
			if title == "" {
				continue
			}
			if result.Title == "" && level == 1 {
				result.Title = title
			}
			result.Outline = append(result.Outline, documentOutlineItem{
				ID: stableTextID("heading", title), Title: title, Level: level,
			})
			continue
		}

		if trimmed == "" {
			flushParagraph()
			continue
		}
		// 分割线和表格分隔行不构成可评论的正文块。
		if isThematicBreak(trimmed) {
			flushParagraph()
			continue
		}
		paragraph = append(paragraph, trimmed)
	}
	if inCode {
		flushCode()
	}
	flushParagraph()

	if result.Title == "" && len(result.Outline) > 0 {
		result.Title = result.Outline[0].Title
	}
	return result
}

// parseATXHeading 解析 ATX 标题，返回层级与去除装饰后的标题文本。
func parseATXHeading(line string) (int, string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, "", false
	}
	// `#` 之后必须是空格，否则不是标题（例如话题标签）。
	if level < len(line) && line[level] != ' ' && line[level] != '\t' {
		return 0, "", false
	}
	title := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(line[level:]), "#"))
	return level, strings.TrimSpace(title), true
}

func isThematicBreak(line string) bool {
	if len(line) < 3 {
		return false
	}
	for _, marker := range []string{"-", "*", "_"} {
		if strings.Trim(line, marker+" ") == "" && strings.Count(line, marker) >= 3 {
			return true
		}
	}
	// Markdown 表格的分隔行，例如 |---|---|
	if strings.HasPrefix(line, "|") && strings.Trim(line, "|-: ") == "" {
		return true
	}
	return false
}

// firstMarkdownParagraph 返回首个段落纯文本，用于生成项目摘要类展示。
func firstMarkdownParagraph(markdown string) string {
	for _, block := range parseMarkdownDocument(markdown).Blocks {
		if block.Type == "paragraph" {
			return block.Text
		}
	}
	return ""
}
