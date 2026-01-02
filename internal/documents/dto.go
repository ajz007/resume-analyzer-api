package documents

import "time"

// DocumentResponse is the outward-facing representation of a document.
type DocumentResponse struct {
	DocumentID string    `json:"documentId"`
	FileName   string    `json:"fileName"`
	MimeType   string    `json:"mimeType"`
	SizeBytes  int64     `json:"sizeBytes"`
	UploadedAt time.Time `json:"uploadedAt"`
}

func toResponse(doc Document) DocumentResponse {
	return DocumentResponse{
		DocumentID: doc.ID,
		FileName:   doc.FileName,
		MimeType:   doc.MimeType,
		SizeBytes:  doc.SizeBytes,
		UploadedAt: doc.CreatedAt,
	}
}
