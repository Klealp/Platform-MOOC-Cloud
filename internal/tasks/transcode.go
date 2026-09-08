package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hibiken/asynq"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/metrics"
	"mooc-platform/internal/queue"
)

// =====================================================================
// Transcodificacion a HLS
//
// QUE ES HLS: en vez de servir un archivo .mp4 gigante, el video se parte en
// segmentos de unos segundos y se genera una lista (.m3u8) que los enumera.
// El reproductor pide los segmentos uno a uno y puede cambiar de calidad
// sobre la marcha segun el ancho de banda. Eso es la "reproduccion
// adaptativa" del enunciado.
//
// REGLAS QUE SE RESPETAN:
//   - El ORIGINAL se conserva siempre (nunca se borra ni se sobrescribe).
//   - No hay UPSCALING: si el video es de 480p no se genera una variante de
//     720p. Subir la resolucion no agrega informacion, solo gasta CPU y
//     almacenamiento.
//   - La operacion es idempotente: si el trabajo se reentrega, el asset ya
//     esta en 'ready' y no se vuelve a transcodificar.
// =====================================================================

// escalera de calidades. Solo se generan las que NO superan la altura original.
var ladder = []struct {
	Height    int
	Bitrate   string
	Bandwidth int
}{
	{1080, "5000k", 5500000},
	{720, "2800k", 3000000},
	{480, "1400k", 1600000},
	{360, "800k", 900000},
}

func (d *Deps) HandleTranscode(ctx context.Context, t *asynq.Task) error {
	var p queue.AssetPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("payload invalido: %w: %w", err, asynq.SkipRetry)
	}
	jobKey := "hls:" + p.AssetID

	done, err := d.alreadyProcessed(ctx, jobKey)
	if err != nil {
		return err
	}
	if done {
		d.skipDuplicate(queue.TaskTranscodeHLS, jobKey)
		return nil
	}

	var kind, status, objectKey, originalName string
	err = d.DB.QueryRowContext(ctx, `
		SELECT kind::text, status::text, object_key, original_name FROM assets WHERE id = $1`, p.AssetID).
		Scan(&kind, &status, &objectKey, &originalName)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("asset %s no existe: %w", p.AssetID, asynq.SkipRetry)
	}
	if err != nil {
		return err
	}
	if status == "ready" {
		d.skipDuplicate(queue.TaskTranscodeHLS, jobKey)
		return nil
	}
	if status != "processing" {
		return fmt.Errorf("asset %s en estado %s, no se transcodifica: %w", p.AssetID, status, asynq.SkipRetry)
	}

	// Espacio de trabajo LOCAL DEL WORKER. La API nunca escribe en disco;
	// el worker si puede, porque su disco es desechable: todo lo que produce
	// termina en el almacenamiento de objetos.
	workDir, err := os.MkdirTemp("", "hls-"+p.AssetID+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	input := filepath.Join(workDir, "original"+filepath.Ext(originalName))
	if err := d.Store.DownloadFile(ctx, objectKey, input); err != nil {
		return err
	}

	duration, height, err := probeMedia(ctx, input)
	if err != nil {
		return d.failTranscode(ctx, p.AssetID, jobKey, err.Error())
	}

	outDir := filepath.Join(workDir, "hls")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	var variants []variantInfo
	if kind == "audio" {
		v, err := transcodeAudio(ctx, input, outDir)
		if err != nil {
			return d.failTranscode(ctx, p.AssetID, jobKey, err.Error())
		}
		variants = append(variants, v)
	} else {
		for _, rung := range ladder {
			if rung.Height > height {
				continue // sin upscaling
			}
			v, err := transcodeVideo(ctx, input, outDir, rung.Height, rung.Bitrate, rung.Bandwidth)
			if err != nil {
				return d.failTranscode(ctx, p.AssetID, jobKey, err.Error())
			}
			variants = append(variants, v)
		}
		if len(variants) == 0 {
			// Video mas pequeno que el escalon mas bajo: se usa su altura real.
			v, err := transcodeVideo(ctx, input, outDir, height, "600k", 700000)
			if err != nil {
				return d.failTranscode(ctx, p.AssetID, jobKey, err.Error())
			}
			variants = append(variants, v)
		}
	}

	if err := writeMasterPlaylist(outDir, variants); err != nil {
		return err
	}

	// Prefijo de los derivados. El original vive en originals/... y no se toca.
	prefix := fmt.Sprintf("hls/%s", p.AssetID)
	if err := d.uploadDir(ctx, outDir, prefix); err != nil {
		return err
	}

	if _, err := d.DB.ExecContext(ctx, `
		UPDATE assets SET status = 'ready', hls_prefix = $1, duration_secs = $2, updated_at = now()
		WHERE id = $3`, prefix, duration, p.AssetID); err != nil {
		return err
	}
	if err := d.markResourcesReady(ctx, p.AssetID); err != nil {
		return err
	}

	metrics.TranscodeMinutes.Add(float64(duration) / 60.0)
	d.Audit.Log(ctx, "", audit.ActionAssetReady, "asset", p.AssetID, "",
		map[string]any{"duration_secs": duration, "variants": len(variants)})
	d.markProcessed(ctx, jobKey, queue.TaskTranscodeHLS,
		map[string]any{"variants": len(variants), "duration_secs": duration})
	return nil
}

func (d *Deps) failTranscode(ctx context.Context, assetID, jobKey, reason string) error {
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
	d.markProcessed(ctx, jobKey, queue.TaskTranscodeHLS, map[string]any{"result": "failed", "reason": reason})
	return nil
}

type variantInfo struct {
	Playlist  string
	Height    int
	Bandwidth int
	AudioOnly bool
}

// probeMedia obtiene duracion y altura reales con ffprobe.
// El dato viene del ARCHIVO, no del cliente: por eso se puede usar para
// decidir cuanto tiempo debe permanecer un estudiante en el recurso.
func probeMedia(ctx context.Context, path string) (durationSecs, height int, err error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("ffprobe fallo: %w", err)
	}

	var probe struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return 0, 0, fmt.Errorf("respuesta de ffprobe ilegible: %w", err)
	}

	if f, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
		durationSecs = int(f)
	}
	for _, st := range probe.Streams {
		if st.CodecType == "video" && st.Height > height {
			height = st.Height
		}
	}
	if durationSecs <= 0 {
		return 0, 0, errors.New("no se pudo determinar la duracion del archivo")
	}
	return durationSecs, height, nil
}

func transcodeVideo(ctx context.Context, input, outDir string, height int, bitrate string, bandwidth int) (variantInfo, error) {
	name := fmt.Sprintf("v%d", height)
	playlist := name + ".m3u8"

	args := []string{
		"-y", "-i", input,
		"-vf", fmt.Sprintf("scale=-2:%d", height),
		"-c:v", "libx264", "-profile:v", "main", "-preset", "veryfast",
		"-b:v", bitrate, "-maxrate", bitrate, "-bufsize", bitrate,
		"-c:a", "aac", "-b:a", "128k", "-ac", "2",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", filepath.Join(outDir, name+"_%03d.ts"),
		filepath.Join(outDir, playlist),
	}
	if err := runFFmpeg(ctx, args); err != nil {
		return variantInfo{}, err
	}
	return variantInfo{Playlist: playlist, Height: height, Bandwidth: bandwidth}, nil
}

func transcodeAudio(ctx context.Context, input, outDir string) (variantInfo, error) {
	playlist := "a128.m3u8"
	args := []string{
		"-y", "-i", input,
		"-vn",
		"-c:a", "aac", "-b:a", "128k", "-ac", "2",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", filepath.Join(outDir, "a128_%03d.ts"),
		filepath.Join(outDir, playlist),
	}
	if err := runFFmpeg(ctx, args); err != nil {
		return variantInfo{}, err
	}
	return variantInfo{Playlist: playlist, Bandwidth: 160000, AudioOnly: true}, nil
}

func runFFmpeg(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return fmt.Errorf("ffmpeg fallo: %w (%s)", err, msg)
	}
	return nil
}

// writeMasterPlaylist genera index.m3u8, la lista maestra que enumera las
// variantes disponibles. Es la que pide primero el reproductor.
func writeMasterPlaylist(outDir string, variants []variantInfo) error {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	for _, v := range variants {
		if v.AudioOnly {
			b.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,CODECS=\"mp4a.40.2\"\n%s\n",
				v.Bandwidth, v.Playlist))
			continue
		}
		b.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=x%d\n%s\n",
			v.Bandwidth, v.Height, v.Playlist))
	}
	return os.WriteFile(filepath.Join(outDir, "index.m3u8"), []byte(b.String()), 0o644)
}

// uploadDir sube al almacenamiento todo lo que produjo ffmpeg.
func (d *Deps) uploadDir(ctx context.Context, dir, prefix string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		local := filepath.Join(dir, e.Name())
		key := prefix + "/" + e.Name()

		contentType := "application/octet-stream"
		switch filepath.Ext(e.Name()) {
		case ".m3u8":
			contentType = "application/vnd.apple.mpegurl"
		case ".ts":
			contentType = "video/mp2t"
		}
		if err := d.Store.UploadFile(ctx, key, local, contentType); err != nil {
			return fmt.Errorf("subir %s: %w", key, err)
		}
	}
	return nil
}
