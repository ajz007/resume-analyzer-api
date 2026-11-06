package analyses

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

type reviewPreviewBlock struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Read the same paragraphs and styles written to the Word document. This keeps
// the web preview aligned with the exported document without displaying raw XML.
func previewDOCXBlocks(data []byte) ([]reviewPreviewBlock, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	file, err := archive.Open("word/document.xml")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var document struct {
		Paragraphs []struct {
			Properties struct {
				Style struct {
					Value string `xml:"val,attr"`
				} `xml:"pStyle"`
				Numbering *struct{} `xml:"numPr"`
			} `xml:"pPr"`
			Runs []struct {
				Text string `xml:"t"`
			} `xml:"r"`
		} `xml:"body>p"`
	}
	if err := xml.NewDecoder(file).Decode(&document); err != nil {
		return nil, err
	}
	blocks := make([]reviewPreviewBlock, 0, len(document.Paragraphs))
	for _, p := range document.Paragraphs {
		var text strings.Builder
		for _, run := range p.Runs {
			text.WriteString(run.Text)
		}
		if text.Len() == 0 {
			continue
		}
		kind := "paragraph"
		switch {
		case p.Properties.Numbering != nil:
			kind = "bullet"
		case p.Properties.Style.Value == "Heading1":
			kind = "heading"
		case p.Properties.Style.Value == "Title":
			kind = "title"
		}
		blocks = append(blocks, reviewPreviewBlock{Kind: kind, Text: text.String()})
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("Word export contains no preview paragraphs")
	}
	return blocks, nil
}
