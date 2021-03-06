package renderer

import (
	"net/http"
	"sync"
)

// D is the syntax-sugar type for map
type D map[string]interface{}

// Data is the container type for renderer data
type Data struct {
	m        sync.RWMutex
	r        *http.Request
	template string
	data     D
}

// NewData sets fields for renderer data
func NewData(r *http.Request, template string, data D) *Data {
	if data == nil {
		data = make(D)
	}

	return &Data{
		r:        r,
		template: template,
		data:     data,
	}
}

// Set sets named value to the data container
func (d *Data) Set(key string, value interface{}) *Data {
	d.m.Lock()
	defer d.m.Unlock()

	d.data[key] = value
	return d
}

// Renderer is the interface for rendering data to a client in required format
type Renderer interface {
	// Render renders the data with response code to a HTTP response writer
	Render(w http.ResponseWriter, code int, data *Data)
}
