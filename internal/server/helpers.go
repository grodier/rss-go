package server

import (
	"bytes"
	"net/http"
)

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		method = r.Method
		uri    = r.URL.RequestURI()
	)

	s.logger.Error(err.Error(), "method", method, "uri", uri)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (s *Server) clientError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, page string, data any) {

	buf := new(bytes.Buffer)

	err := s.template.ExecuteTemplate(buf, page, data)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	w.WriteHeader(status)

	buf.WriteTo(w)
}
