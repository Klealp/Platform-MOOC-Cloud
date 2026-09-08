// Package storage aisla al resto del sistema del proveedor de almacenamiento
// de objetos.
//
// DECISION DE ARQUITECTURA (justificable en el video): ningun handler ni
// worker habla directamente con MinIO. Todos usan este tipo. MinIO implementa
// la API de S3, y Google Cloud Storage expone una API de interoperabilidad
// compatible con S3, de modo que en la Entrega 2 basta cambiar las variables
// de entorno (o anadir un archivo gcs.go con los mismos metodos) sin tocar
// una sola linea del dominio.
package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Storage struct {
	client *minio.Client
	core   *minio.Core // expone las operaciones multipart de bajo nivel
	bucket string
	// signer es un cliente MinIO configurado con el endpoint PUBLICO
	// (el que ve el navegador o curl desde tu maquina, no el nombre interno
	// de Docker). Se usa EXCLUSIVAMENTE para firmar URLs, nunca para hacer
	// peticiones de red: firmar es un calculo local, no requiere que el
	// endpoint sea alcanzable desde dentro del contenedor.
	//
	// Por que hace falta un cliente aparte: en AWS SigV4 (el esquema de firma
	// que usa S3 y que MinIO implementa) el header Host SI forma parte de lo
	// que se firma -- viaja listado en "X-Amz-SignedHeaders=host" dentro de
	// la propia URL. Si se firma con host "minio:9000" y despues se reescribe
	// el texto de la URL a "localhost:9000", la peticion que de verdad llega
	// a MinIO trae un Host distinto al que se uso para calcular la firma, y
	// MinIO la rechaza con 403 (SignatureDoesNotMatch). La unica forma
	// correcta de que el cliente externo pueda usar la URL es firmarla desde
	// el principio contra el host que el va a usar.
	signer *minio.Client
}

type Config struct {
	Endpoint       string
	PublicEndpoint string
	AccessKey      string
	SecretKey      string
	Bucket         string
	Region         string
	UseSSL         bool
}

func New(cfg Config) (*Storage, error) {
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}

	// Cliente INTERNO: para operaciones contenedor-a-contenedor (crear el
	// bucket, ensamblar la carga multipart, leer bytes para el escaneo...).
	// Estas si son conexiones de red reales, y solo "minio:9000" resuelve
	// dentro de la red de Docker.
	core, err := minio.NewCore(cfg.Endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("crear cliente de objetos: %w", err)
	}

	publicEndpoint := cfg.PublicEndpoint
	if publicEndpoint == "" {
		publicEndpoint = cfg.Endpoint
	}
	// Cliente de FIRMA: nunca hace una peticion de red (Presign es un calculo
	// local), asi que no importa que "localhost:9000" no sea alcanzable desde
	// dentro del contenedor de la API.
	signer, err := minio.New(publicEndpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("crear firmante de URLs publicas: %w", err)
	}

	return &Storage{
		client: core.Client,
		core:   core,
		bucket: cfg.Bucket,
		signer: signer,
	}, nil
}

// EnsureBucket crea el bucket si no existe. Se llama al arrancar la API para
// que el sistema quede operativo con un solo `docker compose up`.
func (s *Storage) EnsureBucket(ctx context.Context) error {
	var lastErr error
	for i := 1; i <= 30; i++ {
		exists, err := s.client.BucketExists(ctx, s.bucket)
		if err == nil {
			if exists {
				log.Printf("storage: bucket %q listo", s.bucket)
				return nil
			}
			if err = s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err == nil {
				log.Printf("storage: bucket %q creado", s.bucket)
				return nil
			}
		}
		lastErr = err
		log.Printf("storage: almacenamiento no responde (intento %d/30): %v", i, err)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("no se pudo preparar el bucket: %w", lastErr)
}

func (s *Storage) Bucket() string { return s.bucket }

// ---------------------------------------------------------------------
// Carga multipart directa y reanudable
// ---------------------------------------------------------------------
//
// El archivo NUNCA pasa por la API. El cliente pide un juego de URLs
// prefirmadas, sube cada parte directamente al almacenamiento y despues
// avisa a la API para que cierre la carga. Asi la API sigue sin estado y no
// se convierte en cuello de botella con videos de cientos de megabytes.

// NewMultipartUpload inicia la carga y devuelve el uploadID del proveedor.
func (s *Storage) NewMultipartUpload(ctx context.Context, objectKey, contentType string) (string, error) {
	return s.core.NewMultipartUpload(ctx, s.bucket, objectKey, minio.PutObjectOptions{
		ContentType: contentType,
	})
}

// PresignPartURL firma la subida de UNA parte concreta.
func (s *Storage) PresignPartURL(ctx context.Context, objectKey, uploadID string, partNumber int, ttl time.Duration) (string, error) {
	params := url.Values{}
	params.Set("uploadId", uploadID)
	params.Set("partNumber", strconv.Itoa(partNumber))

	u, err := s.signer.Presign(ctx, http.MethodPut, s.bucket, objectKey, ttl, params)
	if err != nil {
		return "", fmt.Errorf("firmar parte %d: %w", partNumber, err)
	}
	return u.String(), nil
}

// UploadedPart describe una parte ya recibida por el almacenamiento.
type UploadedPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
	Size       int64  `json:"size"`
}

// ListParts pregunta al ALMACENAMIENTO que partes llegaron. Es la fuente
// autoritativa para reanudar: no confiamos en lo que el cliente diga haber
// subido, preguntamos a quien recibio los bytes.
func (s *Storage) ListParts(ctx context.Context, objectKey, uploadID string) ([]UploadedPart, error) {
	var out []UploadedPart
	marker := 0
	for {
		res, err := s.core.ListObjectParts(ctx, s.bucket, objectKey, uploadID, marker, 1000)
		if err != nil {
			return nil, fmt.Errorf("listar partes: %w", err)
		}
		for _, p := range res.ObjectParts {
			out = append(out, UploadedPart{PartNumber: p.PartNumber, ETag: p.ETag, Size: p.Size})
		}
		if !res.IsTruncated {
			break
		}
		marker = res.NextPartNumberMarker
	}
	return out, nil
}

// CompleteMultipartUpload ensambla las partes en el objeto final.
func (s *Storage) CompleteMultipartUpload(ctx context.Context, objectKey, uploadID string, parts []UploadedPart) error {
	cp := make([]minio.CompletePart, 0, len(parts))
	for _, p := range parts {
		cp = append(cp, minio.CompletePart{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	_, err := s.core.CompleteMultipartUpload(ctx, s.bucket, objectKey, uploadID, cp, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("completar carga: %w", err)
	}
	return nil
}

func (s *Storage) AbortMultipartUpload(ctx context.Context, objectKey, uploadID string) error {
	return s.core.AbortMultipartUpload(ctx, s.bucket, objectKey, uploadID)
}

// ---------------------------------------------------------------------
// Operaciones simples
// ---------------------------------------------------------------------

func (s *Storage) PutBytes(ctx context.Context, objectKey string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, objectKey, bytesReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	return err
}

// GetRange descarga los primeros n bytes de un objeto. Se usa para detectar
// el MIME real sin traer un video completo a memoria.
func (s *Storage) GetRange(ctx context.Context, objectKey string, n int64) ([]byte, error) {
	opts := minio.GetObjectOptions{}
	if err := opts.SetRange(0, n-1); err != nil {
		return nil, err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, objectKey, opts)
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

// OpenStream devuelve el objeto completo como lector (para calcular checksum).
func (s *Storage) OpenStream(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	return s.client.GetObject(ctx, s.bucket, objectKey, minio.GetObjectOptions{})
}

// DownloadFile trae el objeto al disco local del worker (no de la API).
func (s *Storage) DownloadFile(ctx context.Context, objectKey, localPath string) error {
	return s.client.FGetObject(ctx, s.bucket, objectKey, localPath, minio.GetObjectOptions{})
}

func (s *Storage) UploadFile(ctx context.Context, objectKey, localPath, contentType string) error {
	_, err := s.client.FPutObject(ctx, s.bucket, objectKey, localPath,
		minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *Storage) StatSize(ctx context.Context, objectKey string) (int64, error) {
	info, err := s.client.StatObject(ctx, s.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return 0, err
	}
	return info.Size, nil
}

func (s *Storage) Remove(ctx context.Context, objectKey string) error {
	return s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{})
}

// PresignGet devuelve una URL de descarga temporal. Es el mecanismo con el que
// se entrega TODO material privado: primero se verifica el derecho de acceso
// en la API y solo entonces se firma la URL. El bucket nunca es publico.
func (s *Storage) PresignGet(ctx context.Context, objectKey string, ttl time.Duration, downloadName string) (string, error) {
	params := url.Values{}
	if downloadName != "" {
		params.Set("response-content-disposition", "attachment; filename=\""+downloadName+"\"")
	}
	u, err := s.signer.PresignedGetObject(ctx, s.bucket, objectKey, ttl, params)
	if err != nil {
		return "", fmt.Errorf("firmar descarga: %w", err)
	}
	return u.String(), nil
}

// bytesReader evita importar "bytes" solo para esto en varios archivos.
func bytesReader(b []byte) io.Reader { return &sliceReader{data: b} }

type sliceReader struct {
	data []byte
	off  int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}
