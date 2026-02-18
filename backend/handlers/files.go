package handlers

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"minecraft-admin/minecraft"
)

// FileHandler handles file browser REST endpoints
type FileHandler struct {
	mgr *minecraft.Manager
}

// NewFileHandler creates a new FileHandler
func NewFileHandler(mgr *minecraft.Manager) *FileHandler {
	return &FileHandler{mgr: mgr}
}

// List handles GET /api/servers/{id}/files?path=subdir
func (h *FileHandler) List(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	subPath := r.URL.Query().Get("path")
	if subPath == "" {
		subPath = "."
	}

	files, err := h.mgr.ListFiles(id, subPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, files)
}

// ReadContent handles GET /api/servers/{id}/files/content?path=file.txt
func (h *FileHandler) ReadContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	subPath := r.URL.Query().Get("path")
	if subPath == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}

	data, err := h.mgr.ReadFileContent(id, subPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

// WriteContent handles PUT /api/servers/{id}/files/content
func (h *FileHandler) WriteContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Path == "" {
		respondError(w, http.StatusBadRequest, "path is required")
		return
	}

	if err := h.mgr.WriteFileContent(id, req.Path, []byte(req.Content)); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// Upload handles POST /api/servers/{id}/files/upload?path=subdir
func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	subPath := r.URL.Query().Get("path")
	if subPath == "" {
		subPath = "."
	}

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "No file provided")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to read uploaded file")
		return
	}

	targetPath := subPath + "/" + header.Filename
	if subPath == "." {
		targetPath = header.Filename
	}
	targetPath, err = h.mgr.ResolveUploadSubPath(id, targetPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.mgr.WriteFileContent(id, targetPath, data); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "uploaded", "name": filepath.Base(targetPath)})
}

// Delete handles DELETE /api/servers/{id}/files?path=file.txt
func (h *FileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	subPath := r.URL.Query().Get("path")
	if subPath == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}

	if err := h.mgr.DeletePath(id, subPath); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// MkDir handles POST /api/servers/{id}/files/mkdir
func (h *FileHandler) MkDir(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Path == "" {
		respondError(w, http.StatusBadRequest, "path is required")
		return
	}

	if err := h.mgr.CreateDirectory(id, req.Path); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

// Rename handles PUT /api/servers/{id}/files/rename
func (h *FileHandler) Rename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		OldPath string `json:"oldPath"`
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.OldPath == "" || req.NewName == "" {
		respondError(w, http.StatusBadRequest, "oldPath and newName are required")
		return
	}

	if err := h.mgr.RenamePath(id, req.OldPath, req.NewName); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
}

// Download handles POST /api/servers/{id}/files/download
// Body: { "paths": ["file1.txt", "dir/file2.txt"] }
// Single file: serves directly. Multiple files: serves as batch.zip
func (h *FileHandler) Download(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.Paths) == 0 {
		respondError(w, http.StatusBadRequest, "At least one path is required")
		return
	}

	// Single file — serve directly
	if len(req.Paths) == 1 {
		absPath, err := h.mgr.GetFilePath(id, req.Paths[0])
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		info, err := os.Stat(absPath)
		if err != nil {
			respondError(w, http.StatusNotFound, "File not found")
			return
		}
		if info.IsDir() {
			respondError(w, http.StatusBadRequest, "Cannot download a directory as a single file")
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(absPath)))
		http.ServeFile(w, r, absPath)
		return
	}

	// Multiple files — create zip
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="batch.zip"`)

	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, p := range req.Paths {
		absPath, err := h.mgr.GetFilePath(id, p)
		if err != nil {
			continue
		}
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() {
			continue
		}

		f, err := os.Open(absPath)
		if err != nil {
			continue
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			f.Close()
			continue
		}
		header.Name = p
		header.Method = zip.Deflate

		writer, err := zw.CreateHeader(header)
		if err != nil {
			f.Close()
			continue
		}
		io.Copy(writer, f)
		f.Close()
	}
}
