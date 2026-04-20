package handler

import (
	"fmt"
	"net/http"

	"git.rwth-aachen.de/dsma/publications/software/konda/view"
)

// IndexHandler handles a view for the index page.
func (h *Handlers) IndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		w.WriteHeader(404)
		view.NotFound(w)
		return
	}

	err := view.Index(w)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}
