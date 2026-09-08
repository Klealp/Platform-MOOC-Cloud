package tasks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hibiken/asynq"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/queue"
)

// eicarSignature es el patron del archivo de prueba EICAR: un texto inofensivo
// que TODOS los antivirus del mundo reconocen como si fuera un virus. Existe
// justamente para poder probar la cadena de deteccion sin usar malware real.
const eicarSignature = `X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`

// mimePermitido por tipo de asset. Rechazar aqui es importante: el cliente
// declara un MIME al iniciar la carga, pero un atacante puede declarar
// "application/pdf" y subir un ejecutable. Lo que vale es lo que dicen los
// primeros bytes del archivo, no lo que dijo el cliente.
var mimePermitido = map[string][]string{
	"video": {"video/"},
	"audio": {"audio/", "video/"}, // un .m4a puede detectarse como video/mp4
	"pdf":   {"application/pdf"},
	"image": {"image/"},
	"file":  {""}, // cualquiera
}

// HandleScanAsset verifica el archivo recien subido.
//
// Tres comprobaciones, en este orden:
//  1. Integridad: el SHA-256 real debe coincidir con el declarado.
//  2. Tipo real: los bytes deben corresponder al tipo anunciado.
//  3. Antimalware: se busca la firma; si aparece, el objeto se BORRA.
//
// Solo si las tres pasan el archivo avanza a 'ready' (o a transcodificacion).
func (d *Deps) HandleScanAsset(ctx context.Context, t *asynq.Task) error {
	var p queue.AssetPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("payload invalido: %w: %w", err, asynq.SkipRetry)
	}
	jobKey := "scan:" + p.AssetID

	done, err := d.alreadyProcessed(ctx, jobKey)
	if err != nil {
		return err
	}
	if done {
		d.skipDuplicate(queue.TaskScanAsset, jobKey)
		return nil
	}

	var (
		ownerID, kind, status, objectKey, declaredChecksum string
	)
	err = d.DB.QueryRowContext(ctx, `
		SELECT owner_id, kind::text, status::text, object_key, checksum_sha256
		FROM assets WHERE id = $1`, p.AssetID).
		Scan(&ownerID, &kind, &status, &objectKey, &declaredChecksum)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("asset %s no existe: %w", p.AssetID, asynq.SkipRetry)
	}
	if err != nil {
		return err
	}

	// Comprobacion de estado: si ya paso de 'uploaded', otro worker lo hizo.
	if status != "uploaded" {
		d.skipDuplicate(queue.TaskScanAsset, jobKey)
		return nil
	}

	if _, err := d.DB.ExecContext(ctx,
		`UPDATE assets SET status = 'scanning', updated_at = now() WHERE id = $1`, p.AssetID); err != nil {
		return err
	}

	// --- 1 y 2: se lee el objeto una sola vez ---
	reader, err := d.Store.OpenStream(ctx, objectKey)
	if err != nil {
		return err
	}
	defer reader.Close()

	hasher := sha256.New()
	head := make([]byte, 0, 512)
	infected := false
	// Ventana solapada: la firma podria quedar partida entre dos bloques.
	var tail []byte
	buf := make([]byte, 256*1024)

	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			hasher.Write(chunk)
			if len(head) < 512 {
				head = append(head, chunk[:min(len(chunk), 512-len(head))]...)
			}
			if !infected {
				window := append(tail, chunk...)
				if bytes.Contains(window, []byte(eicarSignature)) {
					infected = true
				}
				if len(window) > len(eicarSignature) {
					tail = append([]byte{}, window[len(window)-len(eicarSignature):]...)
				} else {
					tail = append([]byte{}, window...)
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	detectedMIME := http.DetectContentType(head)

	if declaredChecksum != "" && declaredChecksum != checksum {
		return d.failAsset(ctx, p.AssetID, jobKey,
			fmt.Sprintf("checksum no coincide (declarado %s, real %s)", declaredChecksum, checksum))
	}

	if d.Cfg.AntimalwareMode != "off" && infected {
		// El objeto se elimina: no queremos conservar un archivo marcado.
		if err := d.Store.Remove(ctx, objectKey); err != nil {
			return err
		}
		if _, err := d.DB.ExecContext(ctx, `
			UPDATE assets SET status = 'infected', detected_mime = $1, checksum_sha256 = $2,
			                  failure_reason = 'archivo rechazado por el escaneo antimalware',
			                  updated_at = now()
			WHERE id = $3`, detectedMIME, checksum, p.AssetID); err != nil {
			return err
		}
		d.Audit.Log(ctx, "", audit.ActionAssetInfected, "asset", p.AssetID, "",
			map[string]any{"owner_id": ownerID, "object_key": objectKey})
		d.markProcessed(ctx, jobKey, queue.TaskScanAsset, map[string]any{"result": "infected"})
		return nil
	}

	if !mimeAceptable(kind, detectedMIME) {
		return d.failAsset(ctx, p.AssetID, jobKey,
			fmt.Sprintf("el contenido real (%s) no corresponde a un archivo de tipo %s", detectedMIME, kind))
	}

	// --- Resultado ---
	if kind == "video" || kind == "audio" {
		// Falta transcodificar: el asset queda en 'processing'.
		if _, err := d.DB.ExecContext(ctx, `
			UPDATE assets SET status = 'processing', detected_mime = $1, checksum_sha256 = $2,
			                  updated_at = now()
			WHERE id = $3`, detectedMIME, checksum, p.AssetID); err != nil {
			return err
		}
		d.markProcessed(ctx, jobKey, queue.TaskScanAsset, map[string]any{"result": "clean", "next": "transcode"})
		return d.Queue.EnqueueTranscode(p.AssetID, d.Cfg.MaxRetries)
	}

	if _, err := d.DB.ExecContext(ctx, `
		UPDATE assets SET status = 'ready', detected_mime = $1, checksum_sha256 = $2, updated_at = now()
		WHERE id = $3`, detectedMIME, checksum, p.AssetID); err != nil {
		return err
	}
	if err := d.markResourcesReady(ctx, p.AssetID); err != nil {
		return err
	}
	d.Audit.Log(ctx, "", audit.ActionAssetReady, "asset", p.AssetID, "",
		map[string]any{"mime": detectedMIME})
	d.markProcessed(ctx, jobKey, queue.TaskScanAsset, map[string]any{"result": "ready"})
	return nil
}

func (d *Deps) failAsset(ctx context.Context, assetID, jobKey, reason string) error {
	if _, err := d.DB.ExecContext(ctx, `
		UPDATE assets SET status = 'failed', failure_reason = $1, updated_at = now()
		WHERE id = $2`, reason, assetID); err != nil {
		return err
	}
	if _, err := d.DB.ExecContext(ctx, `
		UPDATE resources SET processing_status = 'failed', updated_at = now()
		WHERE asset_id = $1`, assetID); err != nil {
		return err
	}
	d.markProcessed(ctx, jobKey, queue.TaskScanAsset, map[string]any{"result": "failed", "reason": reason})
	// Devolver nil evita reintentos inutiles: el archivo no va a mejorar solo.
	return nil
}

func (d *Deps) markResourcesReady(ctx context.Context, assetID string) error {
	_, err := d.DB.ExecContext(ctx, `
		UPDATE resources SET processing_status = 'ready', updated_at = now()
		WHERE asset_id = $1 AND processing_status <> 'not_applicable'`, assetID)
	return err
}

func mimeAceptable(kind, detected string) bool {
	prefixes, ok := mimePermitido[kind]
	if !ok {
		return true
	}
	for _, p := range prefixes {
		if p == "" || strings.HasPrefix(detected, p) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
