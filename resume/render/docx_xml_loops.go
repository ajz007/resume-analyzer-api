package render

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
)

const wmlNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
const relNamespace = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

type xmlNode struct {
	Name     xml.Name
	Attr     []xml.Attr
	Children []*xmlNode
	Text     string
	IsText   bool
}

var xmlHeaderPattern = regexp.MustCompile(`(?s)^\s*(<\?xml[^>]+\?>)`)

func parseXMLDocument(xmlText string) (*xmlNode, string, error) {
	header := ""
	if match := xmlHeaderPattern.FindStringSubmatch(xmlText); len(match) > 0 {
		header = match[1]
		xmlText = strings.TrimSpace(xmlText[len(match[0]):])
	}

	decoder := xml.NewDecoder(strings.NewReader(xmlText))
	var stack []*xmlNode
	var root *xmlNode

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}

		switch t := token.(type) {
		case xml.StartElement:
			node := &xmlNode{Name: t.Name, Attr: t.Attr}
			if len(stack) == 0 {
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			text := string([]byte(t))
			if text == "" {
				continue
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, &xmlNode{IsText: true, Text: text})
		}
	}

	if root == nil {
		return nil, "", errors.New("document.xml has no root element")
	}

	return root, header, nil
}

func encodeXMLDocument(header string, root *xmlNode, rootStart, rootEnd string) (string, error) {
	var buf bytes.Buffer
	if header != "" {
		buf.WriteString(header)
		if !strings.HasSuffix(header, "\n") {
			buf.WriteByte('\n')
		}
	}

	clone := cloneNode(root)
	normalizeXMLNSAttrs(clone)
	applyPrefixMap(clone, prefixMapFromRoot(root))

	required := requiredNamespaceMap(prefixesUsed(clone), root)
	rootStart = ensureRootHasNamespaces(rootStart, required)
	buf.WriteString(rootStart)

	encoder := xml.NewEncoder(&buf)
	for _, child := range clone.Children {
		if err := encodeXMLNode(encoder, child); err != nil {
			return "", err
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", err
	}

	buf.WriteString(rootEnd)
	return buf.String(), nil
}

func encodeXMLFragment(nodes []*xmlNode) (string, error) {
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	for _, node := range nodes {
		if err := encodeXMLNode(encoder, node); err != nil {
			return "", err
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func encodeXMLNode(encoder *xml.Encoder, node *xmlNode) error {
	if node.IsText {
		return encoder.EncodeToken(xml.CharData([]byte(node.Text)))
	}
	start := xml.StartElement{Name: node.Name, Attr: node.Attr}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	for _, child := range node.Children {
		if err := encodeXMLNode(encoder, child); err != nil {
			return err
		}
	}
	return encoder.EncodeToken(start.End())
}

func namespaceAttributes(root *xmlNode) []xml.Attr {
	if root == nil {
		return nil
	}
	out := make([]xml.Attr, 0, len(root.Attr))
	for _, attr := range root.Attr {
		if attr.Name.Space == "xmlns" || attr.Name.Local == "xmlns" || (attr.Name.Space == "" && strings.HasPrefix(attr.Name.Local, "xmlns:")) {
			out = append(out, attr)
		}
	}
	return out
}

func parseXMLFragment(fragment string, namespaceAttrs []xml.Attr) (*xmlNode, error) {
	var builder strings.Builder
	builder.WriteString("<root")
	for _, attr := range namespaceAttrs {
		if attr.Name.Space == "xmlns" {
			if attr.Name.Local == "" {
				builder.WriteString(" xmlns=\"")
				builder.WriteString(attr.Value)
				builder.WriteString("\"")
				continue
			}
			builder.WriteString(" xmlns:")
			builder.WriteString(attr.Name.Local)
			builder.WriteString("=\"")
			builder.WriteString(attr.Value)
			builder.WriteString("\"")
			continue
		}
		if attr.Name.Local == "xmlns" && attr.Name.Space == "" {
			builder.WriteString(" xmlns=\"")
			builder.WriteString(attr.Value)
			builder.WriteString("\"")
			continue
		}
		if attr.Name.Space == "" && strings.HasPrefix(attr.Name.Local, "xmlns:") {
			builder.WriteString(" ")
			builder.WriteString(attr.Name.Local)
			builder.WriteString("=\"")
			builder.WriteString(attr.Value)
			builder.WriteString("\"")
		}
	}
	builder.WriteString(">")
	builder.WriteString(fragment)
	builder.WriteString("</root>")

	root, _, err := parseXMLDocument(builder.String())
	if err != nil {
		return nil, err
	}
	return root, nil
}

func expandLoopXMLFragment(fragment string, name string, items []string, itemToken string, namespaces []xml.Attr) (string, error) {
	root, err := parseXMLFragment(fragment, namespaces)
	if err != nil {
		return "", err
	}
	if err := expandLoopInContainer(root, name, items, itemToken); err != nil {
		return "", err
	}
	return encodeXMLFragment(root.Children)
}

func replaceTokensInXMLFragment(fragment string, replacements map[string]string, namespaces []xml.Attr) (string, error) {
	root, err := parseXMLFragment(fragment, namespaces)
	if err != nil {
		return "", err
	}
	replaceTokensInNode(root, replacements)
	return encodeXMLFragment(root.Children)
}

func findBodyNode(root *xmlNode) *xmlNode {
	if root == nil {
		return nil
	}
	var match *xmlNode
	walkXML(root, func(node *xmlNode) bool {
		if isElement(node, "body") {
			match = node
			return false
		}
		return true
	})
	return match
}

func expandLoopInContainer(container *xmlNode, name string, items []string, itemToken string) error {
	return expandLoopInContainerWithRenderer(container, name, len(items), func(template []*xmlNode, idx int) ([]*xmlNode, error) {
		nodes := cloneNodes(template)
		tmp := &xmlNode{Name: xml.Name{Local: "root"}, Children: nodes}
		replaceTokensInNode(tmp, map[string]string{itemToken: items[idx]})
		return tmp.Children, nil
	})
}

func expandLoopInContainerWithRenderer(container *xmlNode, name string, itemCount int, render func([]*xmlNode, int) ([]*xmlNode, error)) error {
	if container == nil {
		return nil
	}
	container.Children = mergeAdjacentTextNodes(container.Children)
	startTag := "{{#" + name + "}}"
	endTag := "{{/" + name + "}}"

	startIdx := -1
	endIdx := -1
	var startNode *xmlNode
	var endNode *xmlNode

	for idx, child := range container.Children {
		text := nodeTextContent(child)
		if startIdx == -1 && strings.Contains(text, startTag) {
			startIdx = idx
			startNode = child
			continue
		}
		if startIdx != -1 && strings.Contains(text, endTag) {
			endIdx = idx
			endNode = child
			break
		}
	}

	if startIdx == -1 || endIdx == -1 || endIdx < startIdx {
		return nil
	}

	if itemCount == 0 {
		var startKeep *xmlNode
		var endKeep *xmlNode
		if startIdx == endIdx && startNode != nil {
			startKeep = removeTokensFromNode(startNode, startTag, endTag)
		} else {
			startKeep = removeTokensFromNode(startNode, startTag)
			endKeep = removeTokensFromNode(endNode, endTag)
		}

		newChildren := make([]*xmlNode, 0, len(container.Children))
		newChildren = append(newChildren, container.Children[:startIdx]...)
		if startKeep != nil {
			newChildren = append(newChildren, startKeep)
		}
		if endKeep != nil && endIdx != startIdx {
			newChildren = append(newChildren, endKeep)
		}
		newChildren = append(newChildren, container.Children[endIdx+1:]...)
		container.Children = newChildren
		return nil
	}

	var startKeep *xmlNode
	var endKeep *xmlNode
	if startIdx == endIdx && startNode != nil {
		startKeep = removeTokensFromNode(startNode, startTag, endTag)
	} else {
		startKeep = removeTokensFromNode(startNode, startTag)
		endKeep = removeTokensFromNode(endNode, endTag)
	}

	templateNodes := cloneNodes(container.Children[startIdx+1 : endIdx])
	rendered := make([]*xmlNode, 0, itemCount*len(templateNodes))
	for i := 0; i < itemCount; i++ {
		nodes, err := render(templateNodes, i)
		if err != nil {
			return err
		}
		rendered = append(rendered, nodes...)
	}

	newChildren := make([]*xmlNode, 0, len(container.Children)-len(templateNodes)+len(rendered))
	newChildren = append(newChildren, container.Children[:startIdx]...)
	if startKeep != nil {
		newChildren = append(newChildren, startKeep)
	}
	newChildren = append(newChildren, rendered...)
	if endKeep != nil {
		newChildren = append(newChildren, endKeep)
	}
	newChildren = append(newChildren, container.Children[endIdx+1:]...)
	container.Children = newChildren

	return nil
}

func paragraphText(p *xmlNode) string {
	var builder strings.Builder
	for _, node := range collectTextElements(p) {
		builder.WriteString(nodeText(node))
	}
	return builder.String()
}

func nodeTextContent(node *xmlNode) string {
	if node == nil {
		return ""
	}
	if node.IsText {
		return node.Text
	}
	if isElement(node, "p") {
		return paragraphText(node)
	}
	return ""
}

func collectTextElements(node *xmlNode) []*xmlNode {
	out := []*xmlNode{}
	walkXML(node, func(n *xmlNode) bool {
		if isElement(n, "t") {
			out = append(out, n)
		}
		return true
	})
	return out
}

func nodeText(node *xmlNode) string {
	if node.IsText {
		return node.Text
	}
	var builder strings.Builder
	for _, child := range node.Children {
		if child.IsText {
			builder.WriteString(child.Text)
		}
	}
	return builder.String()
}

func setNodeText(node *xmlNode, text string) {
	node.Children = node.Children[:0]
	if text == "" {
		return
	}
	node.Children = append(node.Children, &xmlNode{IsText: true, Text: text})
}

func replaceTokensInParagraph(p *xmlNode, replacements map[string]string) {
	textNodes := collectTextElements(p)
	if len(textNodes) == 0 {
		return
	}
	combined := ""
	for _, node := range textNodes {
		combined += nodeText(node)
	}
	updated := combined
	for token, value := range replacements {
		updated = strings.ReplaceAll(updated, token, value)
	}
	if updated == combined {
		return
	}

	setNodeText(textNodes[0], updated)
	for i := 1; i < len(textNodes); i++ {
		setNodeText(textNodes[i], "")
	}
}

func removeTokenFromParagraph(p *xmlNode, token string) {
	replaceTokensInParagraph(p, map[string]string{token: ""})
}

func replaceTokensInNode(root *xmlNode, replacements map[string]string) {
	walkXML(root, func(n *xmlNode) bool {
		if n.IsText {
			n.Text = replaceTokensInText(n.Text, replacements)
			return true
		}
		if isElement(n, "p") {
			replaceTokensInParagraph(n, replacements)
		}
		return true
	})
}

func cloneNode(node *xmlNode) *xmlNode {
	if node == nil {
		return nil
	}
	cloned := &xmlNode{
		Name:   node.Name,
		Attr:   append([]xml.Attr(nil), node.Attr...),
		Text:   node.Text,
		IsText: node.IsText,
	}
	if len(node.Children) > 0 {
		cloned.Children = make([]*xmlNode, 0, len(node.Children))
		for _, child := range node.Children {
			cloned.Children = append(cloned.Children, cloneNode(child))
		}
	}
	return cloned
}

func cloneNodes(nodes []*xmlNode) []*xmlNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]*xmlNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, cloneNode(node))
	}
	return out
}

func isElement(node *xmlNode, local string) bool {
	if node == nil || node.IsText {
		return false
	}
	if node.Name.Local != local {
		return false
	}
	return node.Name.Space == "" || node.Name.Space == wmlNamespace
}

func replaceTokensInText(text string, replacements map[string]string) string {
	updated := text
	for token, value := range replacements {
		updated = strings.ReplaceAll(updated, token, value)
	}
	return updated
}

func removeTokensFromNode(node *xmlNode, tokens ...string) *xmlNode {
	if node == nil {
		return nil
	}
	replacements := make(map[string]string, len(tokens))
	for _, token := range tokens {
		replacements[token] = ""
	}

	if node.IsText {
		node.Text = replaceTokensInText(node.Text, replacements)
		if strings.TrimSpace(node.Text) == "" {
			return nil
		}
		return node
	}
	if isElement(node, "p") {
		replaceTokensInParagraph(node, replacements)
		if strings.TrimSpace(paragraphText(node)) == "" {
			return nil
		}
		return node
	}
	return node
}

func mergeAdjacentTextNodes(nodes []*xmlNode) []*xmlNode {
	if len(nodes) == 0 {
		return nodes
	}
	merged := make([]*xmlNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.IsText && len(merged) > 0 && merged[len(merged)-1].IsText {
			merged[len(merged)-1].Text += node.Text
			continue
		}
		merged = append(merged, node)
	}
	return merged
}

func enforceHeadingBold(root *xmlNode, headings []string) {
	if root == nil {
		return
	}
	headingSet := make(map[string]struct{}, len(headings))
	for _, heading := range headings {
		headingSet[heading] = struct{}{}
	}
	walkXML(root, func(n *xmlNode) bool {
		if !isElement(n, "p") {
			return true
		}
		text := strings.TrimSpace(paragraphText(n))
		matched := false
		for heading := range headingSet {
			if text == heading || strings.Contains(text, heading) {
				matched = true
				break
			}
		}
		if !matched {
			return true
		}
		for _, child := range n.Children {
			if !isElement(child, "r") {
				continue
			}
			ensureRunBold(child)
		}
		return true
	})
}

func ensureRunBold(run *xmlNode) {
	if run == nil {
		return
	}
	var runProps *xmlNode
	for _, child := range run.Children {
		if isElement(child, "rPr") {
			runProps = child
			break
		}
	}
	if runProps == nil {
		runProps = &xmlNode{Name: xml.Name{Space: wmlNamespace, Local: "rPr"}}
		run.Children = append(run.Children, runProps)
	}
	for _, child := range runProps.Children {
		if isElement(child, "b") {
			return
		}
	}
	runProps.Children = append(runProps.Children, &xmlNode{Name: xml.Name{Space: wmlNamespace, Local: "b"}})
}

func expandHighlightsFallback(container *xmlNode, items []string) {
	if container == nil {
		return
	}

	targetIdx := -1
	for idx, child := range container.Children {
		if !isElement(child, "p") {
			continue
		}
		if strings.Contains(paragraphText(child), "{{HIGHLIGHT_ITEM}}") {
			targetIdx = idx
			break
		}
	}
	if targetIdx == -1 {
		return
	}

	if len(items) == 0 {
		container.Children = append(container.Children[:targetIdx], container.Children[targetIdx+1:]...)
		return
	}

	template := container.Children[targetIdx]
	rendered := make([]*xmlNode, 0, len(items))
	for _, item := range items {
		clone := cloneNode(template)
		replaceTokensInParagraph(clone, map[string]string{"{{HIGHLIGHT_ITEM}}": item})
		rendered = append(rendered, clone)
	}

	newChildren := make([]*xmlNode, 0, len(container.Children)-1+len(rendered))
	newChildren = append(newChildren, container.Children[:targetIdx]...)
	newChildren = append(newChildren, rendered...)
	newChildren = append(newChildren, container.Children[targetIdx+1:]...)
	container.Children = newChildren
}

func prefixMapFromRoot(root *xmlNode) map[string]string {
	if root == nil {
		return nil
	}
	out := make(map[string]string)
	for _, attr := range root.Attr {
		if attr.Name.Space == "xmlns" {
			out[attr.Value] = attr.Name.Local
			continue
		}
		if attr.Name.Space == "" && attr.Name.Local == "xmlns" {
			out[attr.Value] = ""
			continue
		}
		if attr.Name.Space == "" && strings.HasPrefix(attr.Name.Local, "xmlns:") {
			out[attr.Value] = strings.TrimPrefix(attr.Name.Local, "xmlns:")
		}
	}
	return out
}

func namespaceDeclsFromRoot(root *xmlNode) map[string]string {
	if root == nil {
		return nil
	}
	out := make(map[string]string)
	for _, attr := range root.Attr {
		if attr.Name.Space == "xmlns" {
			out[attr.Name.Local] = attr.Value
			continue
		}
		if attr.Name.Space == "" && attr.Name.Local == "xmlns" {
			out[""] = attr.Value
			continue
		}
		if attr.Name.Space == "" && strings.HasPrefix(attr.Name.Local, "xmlns:") {
			out[strings.TrimPrefix(attr.Name.Local, "xmlns:")] = attr.Value
		}
	}
	return out
}

func prefixesUsed(node *xmlNode) map[string]struct{} {
	out := make(map[string]struct{})
	walkXML(node, func(n *xmlNode) bool {
		if n.IsText {
			return true
		}
		if prefix := prefixFromName(n.Name.Local); prefix != "" {
			out[prefix] = struct{}{}
		}
		for _, attr := range n.Attr {
			if prefix := prefixFromName(attr.Name.Local); prefix != "" {
				out[prefix] = struct{}{}
			}
		}
		return true
	})
	return out
}

func prefixFromName(name string) string {
	if name == "" {
		return ""
	}
	if name == "xmlns" || strings.HasPrefix(name, "xmlns:") {
		return ""
	}
	if idx := strings.IndexByte(name, ':'); idx > 0 {
		return name[:idx]
	}
	return ""
}

func requiredNamespaceMap(prefixes map[string]struct{}, root *xmlNode) map[string]string {
	declared := namespaceDeclsFromRoot(root)
	required := make(map[string]string)
	for prefix := range prefixes {
		if uri, ok := declared[prefix]; ok {
			required[prefix] = uri
			continue
		}
		if uri, ok := knownNamespaceURIs[prefix]; ok {
			required[prefix] = uri
		}
	}
	if _, ok := required["w"]; !ok {
		required["w"] = wmlNamespace
	}
	if _, ok := required["r"]; !ok {
		required["r"] = relNamespace
	}
	return required
}

var knownNamespaceURIs = map[string]string{
	"w":   wmlNamespace,
	"r":   relNamespace,
	"a":   "http://schemas.openxmlformats.org/drawingml/2006/main",
	"wp":  "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing",
	"pic": "http://schemas.openxmlformats.org/drawingml/2006/picture",
	"mc":  "http://schemas.openxmlformats.org/markup-compatibility/2006",
	"w14": "http://schemas.microsoft.com/office/word/2010/wordml",
	"w15": "http://schemas.microsoft.com/office/word/2012/wordml",
}

func ensureRootHasNamespaces(rootStart string, required map[string]string) string {
	if len(required) == 0 || rootStart == "" {
		return rootStart
	}
	existing := namespacesFromRootStart(rootStart)
	missing := make([]string, 0, len(required))
	for prefix, uri := range required {
		if current, ok := existing[prefix]; ok && current == uri {
			continue
		}
		if uri != "" {
			missing = append(missing, prefix)
		}
	}
	if len(missing) == 0 {
		return rootStart
	}
	sort.Strings(missing)
	var builder strings.Builder
	for _, prefix := range missing {
		uri := required[prefix]
		if prefix == "" {
			builder.WriteString(" xmlns=\"")
			builder.WriteString(uri)
			builder.WriteString("\"")
			continue
		}
		builder.WriteString(" xmlns:")
		builder.WriteString(prefix)
		builder.WriteString("=\"")
		builder.WriteString(uri)
		builder.WriteString("\"")
	}
	insert := builder.String()
	if insert == "" {
		return rootStart
	}
	if idx := strings.LastIndex(rootStart, "/>"); idx != -1 && idx == len(rootStart)-2 {
		return rootStart[:idx] + insert + rootStart[idx:]
	}
	if idx := strings.LastIndex(rootStart, ">"); idx != -1 {
		return rootStart[:idx] + insert + rootStart[idx:]
	}
	return rootStart
}

var xmlnsAttrPattern = regexp.MustCompile(`\s+xmlns(?::([A-Za-z0-9._-]+))?="([^"]+)"`)

func namespacesFromRootStart(rootStart string) map[string]string {
	out := make(map[string]string)
	matches := xmlnsAttrPattern.FindAllStringSubmatch(rootStart, -1)
	for _, match := range matches {
		prefix := ""
		if len(match) > 1 {
			prefix = match[1]
		}
		if len(match) > 2 {
			out[prefix] = match[2]
		}
	}
	return out
}

func extractRootTags(xmlText string) (string, string, error) {
	startIdx, endIdx, name, err := findRootStartTag(xmlText)
	if err != nil {
		return "", "", err
	}
	rootStart := xmlText[startIdx : endIdx+1]
	endTag := "</" + name + ">"
	endPos := strings.LastIndex(xmlText, endTag)
	if endPos == -1 {
		return "", "", errors.New("root end tag not found")
	}
	rootEnd := xmlText[endPos : endPos+len(endTag)]
	return rootStart, rootEnd, nil
}

func findRootStartTag(xmlText string) (int, int, string, error) {
	i := 0
	for i < len(xmlText) {
		idx := strings.IndexByte(xmlText[i:], '<')
		if idx == -1 {
			return 0, 0, "", errors.New("root start tag not found")
		}
		i += idx
		if strings.HasPrefix(xmlText[i:], "<?") {
			end := strings.Index(xmlText[i:], "?>")
			if end == -1 {
				return 0, 0, "", errors.New("xml header not terminated")
			}
			i += end + 2
			continue
		}
		if strings.HasPrefix(xmlText[i:], "<!--") {
			end := strings.Index(xmlText[i:], "-->")
			if end == -1 {
				return 0, 0, "", errors.New("xml comment not terminated")
			}
			i += end + 3
			continue
		}
		if strings.HasPrefix(xmlText[i:], "<!") {
			end := strings.IndexByte(xmlText[i:], '>')
			if end == -1 {
				return 0, 0, "", errors.New("doctype not terminated")
			}
			i += end + 1
			continue
		}
		break
	}
	start := i
	inQuote := byte(0)
	for i = start + 1; i < len(xmlText); i++ {
		c := xmlText[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inQuote = c
			continue
		}
		if c == '>' {
			name := rootTagName(xmlText[start+1 : i])
			if name == "" {
				return 0, 0, "", errors.New("root tag name missing")
			}
			return start, i, name, nil
		}
	}
	return 0, 0, "", errors.New("root start tag not terminated")
}

func rootTagName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] == '/' {
		return ""
	}
	end := len(raw)
	for i := 0; i < len(raw); i++ {
		if raw[i] == ' ' || raw[i] == '\t' || raw[i] == '\n' || raw[i] == '\r' || raw[i] == '/' {
			end = i
			break
		}
	}
	return raw[:end]
}

func applyPrefixMap(node *xmlNode, prefixes map[string]string) {
	if node == nil || len(prefixes) == 0 {
		return
	}
	if !node.IsText {
		if prefix, ok := prefixes[node.Name.Space]; ok && prefix != "" {
			node.Name.Local = prefix + ":" + node.Name.Local
			node.Name.Space = ""
		}
		for i, attr := range node.Attr {
			if attr.Name.Space == "xmlns" || (attr.Name.Space == "" && attr.Name.Local == "xmlns") || (attr.Name.Space == "" && strings.HasPrefix(attr.Name.Local, "xmlns:")) {
				continue
			}
			if prefix, ok := prefixes[attr.Name.Space]; ok && prefix != "" {
				attr.Name.Local = prefix + ":" + attr.Name.Local
				attr.Name.Space = ""
				node.Attr[i] = attr
			}
		}
	}
	for _, child := range node.Children {
		applyPrefixMap(child, prefixes)
	}
}

func walkXML(node *xmlNode, visit func(*xmlNode) bool) bool {
	if node == nil {
		return true
	}
	if !visit(node) {
		return false
	}
	for _, child := range node.Children {
		if !walkXML(child, visit) {
			return false
		}
	}
	return true
}

func normalizeXMLNSAttrs(node *xmlNode) {
	if node == nil {
		return
	}
	if !node.IsText {
		for i, attr := range node.Attr {
			if attr.Name.Space != "xmlns" {
				continue
			}
			attr.Name.Space = ""
			if attr.Name.Local == "" {
				attr.Name.Local = "xmlns"
			} else {
				attr.Name.Local = "xmlns:" + attr.Name.Local
			}
			node.Attr[i] = attr
		}
	}
	for _, child := range node.Children {
		normalizeXMLNSAttrs(child)
	}
}

func normalizeParagraphNesting(root *xmlNode) {
	if root == nil || root.IsText {
		return
	}
	if len(root.Children) == 0 {
		return
	}
	normalized := make([]*xmlNode, 0, len(root.Children))
	for _, child := range root.Children {
		if child == nil {
			continue
		}
		if isElement(child, "p") {
			lifted := extractDirectBlockChildren(child)
			if len(lifted) > 0 {
				for _, liftedNode := range lifted {
					normalizeParagraphNesting(liftedNode)
					normalized = append(normalized, liftedNode)
				}
			}
			normalizeParagraphNesting(child)
			normalized = append(normalized, child)
			continue
		}
		normalizeParagraphNesting(child)
		normalized = append(normalized, child)
	}
	root.Children = normalized
}

func extractDirectBlockChildren(node *xmlNode) []*xmlNode {
	if node == nil || node.IsText || len(node.Children) == 0 {
		return nil
	}
	lifted := make([]*xmlNode, 0)
	kept := make([]*xmlNode, 0, len(node.Children))
	for _, child := range node.Children {
		if isBlockElement(child) {
			lifted = append(lifted, child)
			continue
		}
		kept = append(kept, child)
	}
	node.Children = kept
	return lifted
}

func removeParagraphs(root *xmlNode, shouldRemove func(*xmlNode) bool) {
	if root == nil || root.IsText || len(root.Children) == 0 {
		return
	}
	filtered := make([]*xmlNode, 0, len(root.Children))
	for _, child := range root.Children {
		if child == nil {
			continue
		}
		if isElement(child, "p") && shouldRemove(child) {
			continue
		}
		removeParagraphs(child, shouldRemove)
		filtered = append(filtered, child)
	}
	root.Children = filtered
}

func isBlockElement(node *xmlNode) bool {
	if node == nil || node.IsText {
		return false
	}
	return isElement(node, "p") || isElement(node, "tbl")
}
