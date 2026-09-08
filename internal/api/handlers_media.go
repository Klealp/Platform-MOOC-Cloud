package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/metrics"
)

// =====================================================================
// Carga multipart directa al almacenamiento
// =====================================================================
//
// FLUJO COMPLETO (el archivo nunca pasa por la API):
//
//	1. POST /uploads          -> la API crea el asset, abre la carga multipart
//	                             y devuelve una URL prefirmada por cada parte.
//	2. PUT <url de la parte>  -> el CLIENTE sube cada parte directo a MinIO.
//	3. GET /uploads/{id}      -> si se interrumpio, dice que partes faltan y
//	                             entrega URLs nuevas para ellas.
//	4. POST /uploads/{id}/complete -> la API ensambla el objeto y encola el
//	                             escaneo antimalware.
//
// Por que asi: mantiene la API sin estado y sin memoria ocupada, permite
// reanudar sin volver a empezar y es el mismo patron que se usara en GCP con
// URLs prefirmadas de Cloud Storage.

const (
	minPartSize     = 8 * 1024 * 1024 // 8 MiB (S3 exige >= 5 MiB salvo la ultima)
	maxPartsPerFile = 100             // limita el tamano de la respuesta JSON
)

type initUploadRequest struct {
	OriginalName   string `json:"original_name" binding:"required"`
	SizeBytes      int64  `json:"size_bytes" binding:"required"`
	DeclaredMIME   string `json:"declared_mime"`
	Kind           string `json:"kind" binding:"required"` // video|audio|pdf|image|file
	ChecksumSHA256 string `json:"checksum_sha256"`
}

var validKinds = map[string]bool{"video": true, "audio": true, "pdf": true, "image": true, "file": true}

func (s *Server) handleInitUpload(c *gin.Context) {
	id := currentUser(c)
	var req initUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requieren original_name, size_bytes y kind.", err.Error())
		return
	}
	if !validKinds[req.Kind] {
		badRequest(c, "kind invalido.", gin.H{"allowed": keysOf(validKinds)})
		return
	}
	if req.SizeBytes <= 0 {
		badRequest(c, "size_bytes debe ser mayor que cero.", nil)
		return
	}
	if req.SizeBytes > s.cfg.MaxUploadBytes {
		fail(c, http.StatusRequestEntityTooLarge, codeTooLarge,
			fmt.Sprintf("El archivo supera el maximo de %d MB.", s.cfg.MaxUploadBytes/1024/1024), nil)
		return
	}

	ctx := c.Request.Context()
	safeName := sanitizeFilename(req.OriginalName)

	// El asset se crea antes de subir nada: necesitamos su id para construir
	// la llave del objeto y para que el cliente pueda consultar el estado.
	var assetID string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO assets (owner_id, kind, status, original_name, declared_mime, size_bytes,
		                    checksum_sha256, object_key)
		VALUES ($1, $2::asset_kind, 'awaiting_upload', $3, $4, $5, $6, '')
		RETURNING id`,
		id.UserID, req.Kind, safeName, req.DeclaredMIME, req.SizeBytes,
		strings.ToLower(req.ChecksumSHA256)).Scan(&assetID)
	if err != nil {
		internalError(c, err)
		return
	}

	// Llave del objeto original. Incluye el propietario para que la
	// separacion por usuario tambien sea visible en el almacenamiento.
	objectKey := fmt.Sprintf("originals/%s/%s/%s", id.UserID, assetID, safeName)

	uploadID, err := s.store.NewMultipartUpload(ctx, objectKey, contentTypeFor(req.DeclaredMIME, safeName))
	if err != nil {
		internalError(c, err)
		return
	}

	partSize, totalParts := planParts(req.SizeBytes)
	expires := time.Now().Add(s.cfg.UploadTTL)

	var uploadRow string
	if err := s.db.QueryRowContext(ctx, `
		INSERT INTO uploads (asset_id, owner_id, upload_id, total_parts, part_size, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		assetID, id.UserID, uploadID, totalParts, partSize, expires).Scan(&uploadRow); err != nil {
		internalError(c, err)
		return
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE assets SET object_key = $1, updated_at = now() WHERE id = $2`, objectKey, assetID); err != nil {
		internalError(c, err)
		return
	}

	urls, err := s.presignParts(ctx, objectKey, uploadID, rangeInts(1, totalParts))
	if err != nil {
		internalError(c, err)
		return
	}

	s.audit.Log(ctx, id.UserID, audit.ActionUploadInit, "asset", assetID, c.ClientIP(),
		map[string]any{"name": safeName, "size": req.SizeBytes, "parts": totalParts})

	c.JSON(http.StatusCreated, gin.H{
		"upload_id":   uploadRow,
		"asset_id":    assetID,
		"object_key":  objectKey,
		"part_size":   partSize,
		"total_parts": totalParts,
		"expires_at":  expires,
		"parts":       urls,
		"instructions": "Sube cada parte con PUT a su url. Guarda el ETag que devuelve cada PUT. " +
			"Si se interrumpe, consulta GET /api/v1/uploads/{upload_id} para saber que falta.",
	})
}

type partURL struct {
	PartNumber int    `json:"part_number"`
	URL        string `json:"url"`
}

func (s *Server) presignParts(ctx context.Context, objectKey, uploadID string, parts []int) ([]partURL, error) {
	out := make([]partURL, 0, len(parts))
	for _, n := range parts {
		u, err := s.store.PresignPartURL(ctx, objectKey, uploadID, n, s.cfg.UploadTTL)
		if err != nil {
			return nil, err
		}
		out = append(out, partURL{PartNumber: n, URL: u})
	}
	return out, nil
}

// uploadRecord es el estado de una carga leido de la base.
type uploadRecord struct {
	ID        string
	AssetID   string
	OwnerID   string
	UploadID  string
	Total     int
	PartSize  int64
	ObjectKey string
	ExpiresAt time.Time
	Completed sql.NullTime
	Aborted   sql.NullTime
}

func (s *Server) loadUpload(ctx context.Context, uploadRowID string) (uploadRecord, error) {
	var u uploadRecord
	err := s.db.QueryRowContext(ctx, `
		SELECT up.id, up.asset_id, up.owner_id, up.upload_id, up.total_parts, up.part_size,
		       a.object_key, up.expires_at, up.completed_at, up.aborted_at
		FROM uploads up JOIN assets a ON a.id = up.asset_id
		WHERE up.id = $1`, uploadRowID).
		Scan(&u.ID, &u.AssetID, &u.OwnerID, &u.UploadID, &u.Total, &u.PartSize,
			&u.ObjectKey, &u.ExpiresAt, &u.Completed, &u.Aborted)
	return u, err
}

// handleUploadStatus es el endpoint que hace REANUDABLE la carga.
// Pregunta al almacenamiento que partes llegaron de verdad y firma URLs
// nuevas solo para las que faltan.
func (s *Server) handleUploadStatus(c *gin.Context) {
	id := currentUser(c)
	ctx := c.Request.Context()

	u, err := s.loadUpload(ctx, c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Carga no encontrada.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if u.OwnerID != id.UserID && id.Role != "admin" {
		notFound(c, "Carga no encontrada.")
		return
	}

	if u.Completed.Valid {
		c.JSON(http.StatusOK, gin.H{"status": "completed", "asset_id": u.AssetID})
		return
	}
	if u.Aborted.Valid {
		c.JSON(http.StatusOK, gin.H{"status": "aborted", "asset_id": u.AssetID})
		return
	}
	if time.Now().After(u.ExpiresAt) {
		conflict(c, "La ventana de 24 horas para reanudar esta carga expiro.", nil)
		return
	}

	received, err := s.store.ListParts(ctx, u.ObjectKey, u.UploadID)
	if err != nil {
		internalError(c, err)
		return
	}
	have := map[int]bool{}
	for _, p := range received {
		have[p.PartNumber] = true
	}
	missing := []int{}
	for n := 1; n <= u.Total; n++ {
		if !have[n] {
			missing = append(missing, n)
		}
	}

	urls, err := s.presignParts(ctx, u.ObjectKey, u.UploadID, missing)
	if err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "in_progress", "asset_id": u.AssetID,
		"total_parts": u.Total, "received_parts": len(received),
		"missing_parts": missing, "parts": urls,
		"expires_at": u.ExpiresAt,
	})
}

// handleCompleteUpload cierra la carga y dispara el procesamiento.
//
// Es idempotente: si el cliente reintenta despues de un timeout, la segunda
// llamada devuelve lo mismo sin volver a ensamblar ni a encolar.
func (s *Server) handleCompleteUpload(c *gin.Context) {
	id := currentUser(c)
	ctx := c.Request.Context()

	u, err := s.loadUpload(ctx, c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Carga no encontrada.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if u.OwnerID != id.UserID && id.Role != "admin" {
		notFound(c, "Carga no encontrada.")
		return
	}
	if u.Completed.Valid {
		c.JSON(http.StatusOK, gin.H{"status": "completed", "asset_id": u.AssetID, "already_completed": true})
		return
	}

	received, err := s.store.ListParts(ctx, u.ObjectKey, u.UploadID)
	if err != nil {
		internalError(c, err)
		return
	}
	if len(received) != u.Total {
		unprocessable(c, "Faltan partes por subir.",
			gin.H{"expected": u.Total, "received": len(received)})
		return
	}

	if err := s.store.CompleteMultipartUpload(ctx, u.ObjectKey, u.UploadID, received); err != nil {
		internalError(c, err)
		return
	}

	size, err := s.store.StatSize(ctx, u.ObjectKey)
	if err != nil {
		internalError(c, err)
		return
	}
	metrics.UploadBytes.Add(float64(size))

	partsJSON, _ := json.Marshal(received)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE uploads SET completed_at = now(), parts = $1 WHERE id = $2`, string(partsJSON), u.ID); err != nil {
		internalError(c, err)
		return
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE assets SET status = 'uploaded', size_bytes = $1, updated_at = now() WHERE id = $2`,
		size, u.AssetID); err != nil {
		internalError(c, err)
		return
	}

	// El trabajo pesado empieza AQUI, fuera de la peticion HTTP.
	if err := s.queue.EnqueueScan(u.AssetID, s.cfg.MaxRetries); err != nil {
		internalError(c, err)
		return
	}

	s.audit.Log(ctx, id.UserID, audit.ActionUploadCompleted, "asset", u.AssetID, c.ClientIP(),
		map[string]any{"size": size, "parts": len(received)})

	c.JSON(http.StatusAccepted, gin.H{
		"status": "uploaded", "asset_id": u.AssetID, "size_bytes": size,
		"next": "El archivo se esta verificando. Consulta GET /api/v1/assets/" + u.AssetID,
	})
}

func (s *Server) handleAbortUpload(c *gin.Context) {
	id := currentUser(c)
	ctx := c.Request.Context()

	u, err := s.loadUpload(ctx, c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Carga no encontrada.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if u.OwnerID != id.UserID && id.Role != "admin" {
		notFound(c, "Carga no encontrada.")
		return
	}
	if err := s.store.AbortMultipartUpload(ctx, u.ObjectKey, u.UploadID); err != nil {
		internalError(c, err)
		return
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE uploads SET aborted_at = now() WHERE id = $1`, u.ID); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Carga cancelada."})
}

// =====================================================================
// Consulta y entrega de archivos
// =====================================================================

type assetInfo struct {
	ID            string
	OwnerID       string
	Kind          string
	Status        string
	OriginalName  string
	DetectedMIME  string
	SizeBytes     int64
	ObjectKey     string
	HLSPrefix     string
	DurationSecs  int
	FailureReason string
}

func (s *Server) loadAsset(ctx context.Context, assetID string) (assetInfo, error) {
	var a assetInfo
	err := s.db.QueryRowContext(ctx, `
		SELECT id, owner_id, kind::text, status::text, original_name, detected_mime,
		       size_bytes, object_key, hls_prefix, duration_secs, failure_reason
		FROM assets WHERE id = $1`, assetID).
		Scan(&a.ID, &a.OwnerID, &a.Kind, &a.Status, &a.OriginalName, &a.DetectedMIME,
			&a.SizeBytes, &a.ObjectKey, &a.HLSPrefix, &a.DurationSecs, &a.FailureReason)
	return a, err
}

// canAccessAsset concentra la regla de acceso a material privado.
//
// Tienen acceso: el propietario, un administrador, o un estudiante con
// inscripcion ACTIVA en un curso PUBLICADO cuya version vigente incluye un
// recurso VISIBLE que apunta a este archivo. Cualquier otro caso recibe 404.
func (s *Server) canAccessAsset(ctx context.Context, a assetInfo, userID, role string) (bool, error) {
	if role == "admin" || a.OwnerID == userID {
		return true, nil
	}
	var allowed bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM resources r
			JOIN units u  ON u.id = r.unit_id
			JOIN modules m ON m.id = u.module_id
			JOIN course_versions v ON v.id = m.version_id
			JOIN courses co ON co.id = v.course_id AND co.current_version_id = v.id
			JOIN enrollments e ON e.course_id = co.id
			WHERE r.asset_id = $1
			  AND r.visible
			  AND co.status = 'published'
			  AND e.user_id = $2
			  AND e.status = 'active'
		)`, a.ID, userID).Scan(&allowed)
	return allowed, err
}

func (s *Server) handleGetAsset(c *gin.Context) {
	id := currentUser(c)
	ctx := c.Request.Context()

	a, err := s.loadAsset(ctx, c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Archivo no encontrado.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	ok, err := s.canAccessAsset(ctx, a, id.UserID, id.Role)
	if err != nil {
		internalError(c, err)
		return
	}
	if !ok {
		notFound(c, "Archivo no encontrado.")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id": a.ID, "kind": a.Kind, "status": a.Status,
		"original_name": a.OriginalName, "detected_mime": a.DetectedMIME,
		"size_bytes": a.SizeBytes, "duration_secs": a.DurationSecs,
		"hls_available":  a.HLSPrefix != "",
		"failure_reason": a.FailureReason,
	})
}

// handleAssetDeliveryURL entrega el material SIEMPRE con URL firmada y
// temporal, despues de verificar el derecho de acceso. El bucket nunca es
// publico: sin firma no hay descarga posible.
func (s *Server) handleAssetDeliveryURL(c *gin.Context) {
	id := currentUser(c)
	ctx := c.Request.Context()

	a, err := s.loadAsset(ctx, c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Archivo no encontrado.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	ok, err := s.canAccessAsset(ctx, a, id.UserID, id.Role)
	if err != nil {
		internalError(c, err)
		return
	}
	if !ok {
		notFound(c, "Archivo no encontrado.")
		return
	}
	if a.Status != "ready" {
		conflict(c, "El archivo aun no esta disponible.", gin.H{"status": a.Status})
		return
	}

	out := gin.H{"asset_id": a.ID, "kind": a.Kind, "expires_in": int(s.cfg.SignedURLTTL.Seconds())}

	// Reproduccion adaptativa: si hay derivado HLS, se entrega la lista de
	// reproduccion (que a su vez lleva URLs firmadas para cada segmento).
	if a.HLSPrefix != "" {
		out["playlist_url"] = s.cfg.PublicBaseURL + "/api/v1/assets/" + a.ID + "/playlist"
		out["duration_secs"] = a.DurationSecs
	}

	// El original se conserva siempre y se entrega si el recurso es descargable.
	url, err := s.signDelivery(ctx, a.ObjectKey, a.OriginalName)
	if err != nil {
		internalError(c, err)
		return
	}
	out["download_url"] = url

	c.JSON(http.StatusOK, out)
}

// signDelivery decide entre CDN y URL prefirmada.
// En local (Entrega 1) siempre firma contra MinIO. En la nube (Entrega 2),
// con CDN_BASE_URL configurado, el contenido saldra por la red de
// distribucion en vez de golpear el almacenamiento en cada peticion.
func (s *Server) signDelivery(ctx context.Context, objectKey, downloadName string) (string, error) {
	if s.cfg.CDNBaseURL != "" {
		return strings.TrimRight(s.cfg.CDNBaseURL, "/") + "/" + objectKey, nil
	}
	return s.store.PresignGet(ctx, objectKey, s.cfg.SignedURLTTL, downloadName)
}

// handleHLSPlaylist devuelve la lista de reproduccion con una URL firmada por
// cada segmento.
//
// Por que hace falta: una URL prefirmada ampara UN objeto. Si firmaramos solo
// el .m3u8, el reproductor pediria despues los .ts y recibiria 403. En vez de
// abrir el bucket, reescribimos la lista en el momento de servirla y cada
// segmento viaja con su propia firma temporal.
func (s *Server) handleHLSPlaylist(c *gin.Context) {
	id := currentUser(c)
	ctx := c.Request.Context()

	a, err := s.loadAsset(ctx, c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Archivo no encontrado.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	ok, err := s.canAccessAsset(ctx, a, id.UserID, id.Role)
	if err != nil {
		internalError(c, err)
		return
	}
	if !ok || a.HLSPrefix == "" {
		notFound(c, "Archivo no encontrado.")
		return
	}

	name := c.DefaultQuery("file", "index.m3u8")
	if !safePlaylistName.MatchString(name) {
		badRequest(c, "Nombre de lista invalido.", nil)
		return
	}

	raw, err := s.store.GetRange(ctx, a.HLSPrefix+"/"+name, 1<<20)
	if err != nil {
		notFound(c, "Lista de reproduccion no encontrada.")
		return
	}

	rewritten, err := s.rewritePlaylist(ctx, a, string(raw))
	if err != nil {
		internalError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/vnd.apple.mpegurl", []byte(rewritten))
}

var safePlaylistName = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.m3u8$`)

func (s *Server) rewritePlaylist(ctx context.Context, a assetInfo, body string) (string, error) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasSuffix(trimmed, ".m3u8") {
			// Lista anidada: se pide de nuevo a este mismo endpoint para que
			// tambien se le reescriban sus segmentos.
			lines[i] = s.cfg.PublicBaseURL + "/api/v1/assets/" + a.ID + "/playlist?file=" + trimmed
			continue
		}
		url, err := s.signDelivery(ctx, a.HLSPrefix+"/"+trimmed, "")
		if err != nil {
			return "", err
		}
		lines[i] = url
	}
	return strings.Join(lines, "\n"), nil
}

// =====================================================================
// Utilidades
// =====================================================================

// planParts elige un tamano de parte que mantenga el numero de partes acotado.
func planParts(size int64) (partSize int64, totalParts int) {
	partSize = int64(minPartSize)
	if size/partSize+1 > int64(maxPartsPerFile) {
		partSize = size/int64(maxPartsPerFile) + 1
	}
	totalParts = int(size / partSize)
	if size%partSize != 0 {
		totalParts++
	}
	if totalParts == 0 {
		totalParts = 1
	}
	return partSize, totalParts
}

func rangeInts(from, to int) []int {
	out := make([]int, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// sanitizeFilename evita que un nombre malicioso (por ejemplo "../../etc/x")
// se convierta en una ruta arbitraria dentro del bucket.
func sanitizeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = unsafeChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._")
	if name == "" {
		name = "archivo"
	}
	if len(name) > 120 {
		name = name[len(name)-120:]
	}
	return name
}

func contentTypeFor(declared, filename string) string {
	if declared != "" {
		return declared
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}
