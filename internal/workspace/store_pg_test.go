package workspace

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPGSearchScopesAndEscapesLiteralQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`FROM analyses a[\s\S]*WHERE a.user_id=\$1 AND a.deleted_at IS NULL[\s\S]*LIMIT \$4 OFFSET \$5`).WithArgs("owner", "100%_", `%100\%\_%`, 20, 0).WillReturnRows(sqlmock.NewRows([]string{"id", "doc", "file", "job", "mode", "status", "date", "company", "role"}).AddRow("id", "doc", "file.pdf", "job", "job_match", "completed", time.Now(), "Company", "Engineer"))
	apps, err := (&PGStore{DB: db}).List(context.Background(), "owner", "100%_", 20, 0)
	if err != nil || len(apps) != 1 || apps[0].CompanyName != "Company" {
		t.Fatal(apps, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestPGPrimarySelectionChecksOwnerInOneStatement(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec(`INSERT INTO document_preferences[\s\S]*FROM documents WHERE user_id=\$1 AND id=\$2 AND deleted_at IS NULL[\s\S]*ON CONFLICT`).WithArgs("other", "foreign-document").WillReturnResult(sqlmock.NewResult(0, 0))
	if err = (&PGStore{DB: db}).SetPrimary(context.Background(), "other", "foreign-document"); err != ErrNotFound {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
