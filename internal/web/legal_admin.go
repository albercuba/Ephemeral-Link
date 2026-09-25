package web

import (
	"net/http"
	"strings"

	"ephemeral-link/internal/redisstore"
)

const maxLegalDocumentSize = 256 * 1024

func (a *App) adminSaveLegal(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	privacy := r.FormValue("privacy")
	terms := r.FormValue("terms")
	if len(privacy) > maxLegalDocumentSize || len(terms) > maxLegalDocumentSize {
		http.Error(w, "legal document is too large", http.StatusRequestEntityTooLarge)
		return
	}
	if strings.TrimSpace(privacy) == "" || strings.TrimSpace(terms) == "" {
		http.Error(w, "both legal documents are required", http.StatusBadRequest)
		return
	}
	if err := a.store.SaveLegalDocuments(r.Context(), redisstore.LegalDocuments{Privacy: privacy, Terms: terms}); err != nil {
		a.bad(w, r, err)
		return
	}
	a.audit(r, "admin_save_legal_documents", "privacy_terms", "success", "")
	http.Redirect(w, r, "/admin?saved=legal#legal", http.StatusSeeOther)
}
