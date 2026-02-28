package handlers

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"minecraft-admin/minecraft"
)

// FileHandler handles file browser REST endpoints
type FileHandler struct {
	mgr *minecraft.Manager
}

// Exists handles GET /api/servers/{id}/files/exists?path=file.txt
func (h *FileHandler) Exists(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	subPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if subPath == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}

	absPath, err := h.mgr.GetFilePath(id, subPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	info, statErr := os.Stat(absPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			respondJSON(w, http.StatusOK, map[string]any{
				"exists": false,
			})
			return
		}
		respondError(w, http.StatusInternalServerError, statErr.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"exists": true,
		"isDir":  info.IsDir(),
		"path":   subPath,
	})
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

	relativePath := filepath.ToSlash(filepath.Clean(r.FormValue("relativePath")))
	if relativePath == "." || relativePath == "/" {
		relativePath = ""
	}
	if relativePath == "" {
		relativePath = header.Filename
	}

	targetPath := subPath + "/" + relativePath
	if subPath == "." {
		targetPath = relativePath
	}

	absPath, err := h.mgr.GetFilePath(id, targetPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	conflictAction := strings.ToLower(strings.TrimSpace(r.FormValue("conflictAction")))
	existingInfo, statErr := os.Stat(absPath)
	if statErr == nil {
		if existingInfo.IsDir() {
			respondError(w, http.StatusBadRequest, "cannot replace directory with file")
			return
		}
		if conflictAction == "skip" {
			respondJSON(w, http.StatusOK, map[string]string{
				"status": "skipped",
				"name":   filepath.Base(targetPath),
			})
			return
		}
		if conflictAction != "replace" {
			respondJSON(w, http.StatusConflict, map[string]string{
				"error": "file_exists",
				"name":  filepath.Base(targetPath),
				"path":  targetPath,
			})
			return
		}
	} else if !os.IsNotExist(statErr) {
		respondError(w, http.StatusInternalServerError, statErr.Error())
		return
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := os.WriteFile(absPath, data, 0644); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	status := "uploaded"
	if conflictAction == "replace" {
		status = "replaced"
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": status, "name": filepath.Base(targetPath)})
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
// Single file: serves directly. Directories or multiple paths: serves as zip.
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

	// Keep direct file response for single regular files.
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
		if !info.IsDir() {
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(absPath)))
			http.ServeFile(w, r, absPath)
			return
		}
	}

	w.Header().Set("Content-Type", "application/zip")
	zipName := "batch.zip"
	if len(req.Paths) == 1 {
		zipName = fmt.Sprintf("%s.zip", filepath.Base(req.Paths[0]))
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, zipName))

	zw := zip.NewWriter(w)
	defer zw.Close()

	added := 0
	for _, p := range req.Paths {
		absPath, err := h.mgr.GetFilePath(id, p)
		if err != nil {
			continue
		}
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			_ = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() {
					return nil
				}
				relPath, relErr := filepath.Rel(absPath, path)
				if relErr != nil {
					return nil
				}
				zipPath := filepath.ToSlash(filepath.Join(p, relPath))
				if zipPath == "" {
					return nil
				}
				if err := addPathToZip(zw, path, zipPath); err == nil {
					added++
				}
				return nil
			})
			continue
		}

		if err := addPathToZip(zw, absPath, p); err == nil {
			added++
		}
	}

	if added == 0 {
		respondError(w, http.StatusBadRequest, "No downloadable files found in selected paths")
		return
	}
}

func addPathToZip(zw *zip.Writer, absPath, zipPath string) error {
	f, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(zipPath)
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, f)
	return err
}
