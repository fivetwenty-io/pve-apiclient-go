// Package stream provides response streaming functionality for the PVE API client.
package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Stream represents a streaming response handler.
type Stream struct {
	reader     io.ReadCloser
	decoder    Decoder
	buffer     *bufio.Reader
	config     *Config
	closed     bool
	mu         sync.RWMutex
	metrics    *Metrics
	errorChan  chan error
	cancelFunc context.CancelFunc
}

// Config represents stream configuration.
type Config struct {
	// BufferSize is the size of the read buffer.
	BufferSize int

	// MaxItemSize is the maximum size of a single item.
	MaxItemSize int

	// ReadTimeout is the timeout for read operations.
	ReadTimeout time.Duration

	// Format is the expected stream format (json, jsonlines, csv).
	Format string

	// Delimiter is used for delimited formats.
	Delimiter string
}

// DefaultConfig returns the default stream configuration.
func DefaultConfig() *Config {
	return &Config{
		BufferSize:  4096,
		MaxItemSize: 1024 * 1024, // 1MB
		ReadTimeout: 30 * time.Second,
		Format:      "jsonlines",
		Delimiter:   "\n",
	}
}

// Metrics contains stream metrics.
type Metrics struct {
	ItemsRead    int64
	BytesRead    int64
	ErrorCount   int64
	ReadTime     time.Duration
	LastReadTime time.Time
	mu           sync.RWMutex
}

// Decoder interface for decoding streamed data.
type Decoder interface {
	Decode(data []byte) (interface{}, error)
	SupportsPartial() bool
}

// JSONDecoder decodes JSON data.
type JSONDecoder struct{}

func (d *JSONDecoder) Decode(data []byte) (interface{}, error) {
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *JSONDecoder) SupportsPartial() bool {
	return false
}

// JSONLinesDecoder decodes JSON Lines format.
type JSONLinesDecoder struct{}

func (d *JSONLinesDecoder) Decode(data []byte) (interface{}, error) {
	// Trim whitespace
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return nil, nil
	}

	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *JSONLinesDecoder) SupportsPartial() bool {
	return true
}

// New creates a new stream from an io.ReadCloser.
func New(reader io.ReadCloser, config *Config) *Stream {
	if config == nil {
		config = DefaultConfig()
	}

	// Select decoder based on format
	var decoder Decoder
	switch config.Format {
	case "json":
		decoder = &JSONDecoder{}
	case "jsonlines":
		decoder = &JSONLinesDecoder{}
	default:
		decoder = &JSONLinesDecoder{}
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Stream{
		reader:     reader,
		decoder:    decoder,
		buffer:     bufio.NewReaderSize(reader, config.BufferSize),
		config:     config,
		metrics:    &Metrics{},
		errorChan:  make(chan error, 1),
		cancelFunc: cancel,
	}

	// Start metrics collector if needed
	go s.collectMetrics(ctx)

	return s
}

// NewFromResponse creates a stream from an HTTP response.
func NewFromResponse(resp *http.Response, config *Config) *Stream {
	return New(resp.Body, config)
}

// Read reads the next item from the stream.
func (s *Stream) Read() (interface{}, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("stream is closed")
	}
	s.mu.RUnlock()

	start := time.Now()
	defer func() {
		s.metrics.mu.Lock()
		s.metrics.ReadTime += time.Since(start)
		s.metrics.LastReadTime = time.Now()
		s.metrics.mu.Unlock()
	}()

	// Read based on format
	var data []byte
	var err error

	if s.config.Format == "jsonlines" || s.decoder.SupportsPartial() {
		// Read line by line
		data, err = s.buffer.ReadBytes('\n')
		if err != nil && err != io.EOF {
			s.recordError(err)
			return nil, err
		}
	} else {
		// Read entire content
		data, err = io.ReadAll(s.buffer)
		if err != nil {
			s.recordError(err)
			return nil, err
		}
	}

	// Check size limit
	if len(data) > s.config.MaxItemSize {
		err := fmt.Errorf("item size %d exceeds maximum %d", len(data), s.config.MaxItemSize)
		s.recordError(err)
		return nil, err
	}

	// Update metrics
	s.metrics.mu.Lock()
	s.metrics.BytesRead += int64(len(data))
	s.metrics.mu.Unlock()

	// Decode
	item, decodeErr := s.decoder.Decode(data)
	if decodeErr != nil {
		s.recordError(decodeErr)
		return nil, decodeErr
	}

	// Update metrics
	s.metrics.mu.Lock()
	s.metrics.ItemsRead++
	s.metrics.mu.Unlock()

	return item, err
}

// ReadAll reads all items from the stream.
func (s *Stream) ReadAll() ([]interface{}, error) {
	var items []interface{}

	for {
		item, err := s.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return items, err
		}
		if item != nil {
			items = append(items, item)
		}
	}

	return items, nil
}

// ReadN reads up to n items from the stream.
func (s *Stream) ReadN(n int) ([]interface{}, error) {
	items := make([]interface{}, 0, n)

	for i := 0; i < n; i++ {
		item, err := s.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return items, err
		}
		if item != nil {
			items = append(items, item)
		}
	}

	return items, nil
}

// Channel returns a channel that yields items from the stream.
func (s *Stream) Channel(ctx context.Context) <-chan interface{} {
	ch := make(chan interface{})

	go func() {
		defer close(ch)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				item, err := s.Read()
				if err == io.EOF {
					return
				}
				if err != nil {
					// Send error to error channel
					select {
					case s.errorChan <- err:
					default:
					}
					return
				}
				if item != nil {
					select {
					case ch <- item:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch
}

// Process processes items with a callback function.
func (s *Stream) Process(ctx context.Context, fn func(interface{}) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			item, err := s.Read()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if item != nil {
				if err := fn(item); err != nil {
					return err
				}
			}
		}
	}
}

// ProcessBatch processes items in batches.
func (s *Stream) ProcessBatch(ctx context.Context, batchSize int, fn func([]interface{}) error) error {
	batch := make([]interface{}, 0, batchSize)

	for {
		select {
		case <-ctx.Done():
			// Process remaining batch
			if len(batch) > 0 {
				return fn(batch)
			}
			return ctx.Err()
		default:
			item, err := s.Read()
			if err == io.EOF {
				// Process final batch
				if len(batch) > 0 {
					return fn(batch)
				}
				return nil
			}
			if err != nil {
				return err
			}
			if item != nil {
				batch = append(batch, item)
				if len(batch) >= batchSize {
					if err := fn(batch); err != nil {
						return err
					}
					batch = make([]interface{}, 0, batchSize)
				}
			}
		}
	}
}

// Errors returns the error channel.
func (s *Stream) Errors() <-chan error {
	return s.errorChan
}

// Metrics returns the current stream metrics.
func (s *Stream) Metrics() Metrics {
	s.metrics.mu.RLock()
	defer s.metrics.mu.RUnlock()

	return Metrics{
		ItemsRead:    s.metrics.ItemsRead,
		BytesRead:    s.metrics.BytesRead,
		ErrorCount:   s.metrics.ErrorCount,
		ReadTime:     s.metrics.ReadTime,
		LastReadTime: s.metrics.LastReadTime,
	}
}

// Close closes the stream.
func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	s.cancelFunc()
	close(s.errorChan)
	return s.reader.Close()
}

func (s *Stream) recordError(err error) {
	s.metrics.mu.Lock()
	s.metrics.ErrorCount++
	s.metrics.mu.Unlock()

	// Try to send error to channel
	select {
	case s.errorChan <- err:
	default:
	}
}

func (s *Stream) collectMetrics(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Could emit metrics here if needed
		}
	}
}

// Reader provides a reader interface for streaming.
type Reader struct {
	stream *Stream
}

// NewReader creates a new stream reader.
func NewReader(stream *Stream) *Reader {
	return &Reader{stream: stream}
}

// Read implements io.Reader interface.
func (r *Reader) Read(p []byte) (n int, err error) {
	item, err := r.stream.Read()
	if err != nil {
		return 0, err
	}

	// Convert item to bytes
	data, err := json.Marshal(item)
	if err != nil {
		return 0, err
	}

	n = copy(p, data)
	return n, nil
}

// Transform applies a transformation function to stream items.
type Transform struct {
	stream *Stream
	fn     func(interface{}) (interface{}, error)
}

// NewTransform creates a new transform stream.
func NewTransform(stream *Stream, fn func(interface{}) (interface{}, error)) *Transform {
	return &Transform{
		stream: stream,
		fn:     fn,
	}
}

// Read reads and transforms the next item.
func (t *Transform) Read() (interface{}, error) {
	item, err := t.stream.Read()
	if err != nil {
		return nil, err
	}

	if item == nil {
		return nil, nil
	}

	return t.fn(item)
}

// Filter filters stream items based on a predicate.
type Filter struct {
	stream    *Stream
	predicate func(interface{}) bool
}

// NewFilter creates a new filter stream.
func NewFilter(stream *Stream, predicate func(interface{}) bool) *Filter {
	return &Filter{
		stream:    stream,
		predicate: predicate,
	}
}

// Read reads the next item that matches the filter.
func (f *Filter) Read() (interface{}, error) {
	for {
		item, err := f.stream.Read()
		if err != nil {
			return nil, err
		}

		if item != nil && f.predicate(item) {
			return item, nil
		}
	}
}
