// HTTP server.
package server

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

const baseURL = "https://vimm.net"

var httpClient = &http.Client{}

// Where to put the roms based on their extension.
var paths = map[string]string{
	"smc": "/home/pi/RetroPie/roms/snes/",
	"sfc": "/home/pi/RetroPie/roms/snes/",
}

const js = `
<script>
window.addEventListener('DOMContentLoaded', () => {
	const form = document.querySelector('#download_form');
	form.action = '/download';
});
</script>
`

type handler func(http.ResponseWriter, *http.Request) error

type Server struct {
	host string
	port int
}

func New(host string, port int) *Server {
	return &Server{
		host: host,
		port: port,
	}
}

// GET /
// Request headers and path are used to forwarded as a request to baseURL. The
// response from baseURL is then forwarded.
func (server *Server) index(w http.ResponseWriter, r *http.Request) {
	fwdTo := baseURL + r.URL.String()

	// Create a new request.
	req, err := http.NewRequest("GET", fwdTo, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Add original request headers.
	for k, values := range r.Header {
		// Skip adding this otherwise the html is gzip'd and I can't inject JS into
		// it.
		if k == "Accept-Encoding" {
			continue
		}
		for _, v := range values {
			req.Header.Set(k, v)
		}
	}

	// Request content
	resp, err := httpClient.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	isHTML := resp.Header.Get("Content-Type") == "text/html; charset=UTF-8"

	// Forward headers
	for k, values := range resp.Header {
		for _, v := range values {
			w.Header().Set(k, v)
		}
	}

	if strings.HasPrefix(r.URL.String(), "/vault") && isHTML {
		w.Write(append(body, []byte(js)...))
		return
	}

	w.Write(body)
}

// Returns the file extension if there is one.
func getExt(path string) string {
	split := strings.Split(path, ".")
	return split[len(split)-1]
}

// Unzips the file to the roms directory.
func unzip(path string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}

	defer reader.Close()

	// Iterate through files and unzip rom files to the right directories.
	for _, file := range reader.File {
		ext := getExt(file.FileHeader.Name)
		// relPath is the location on the rasp pi store the ROM.
		if relPath, has := paths[ext]; has {
			rom, err := file.Open()
			if err != nil {
				return err
			}

			defer rom.Close()

			fullPath := relPath + file.FileHeader.Name
			dest, err := os.OpenFile(fullPath, os.O_CREATE|os.O_RDWR, 0755)
			if err != nil {
				return err
			}

			defer dest.Close()

			if _, err := io.Copy(dest, rom); err != nil {
				return err
			}

			log.Printf("SAVED to: %v", fullPath)
		}
	}

	return nil
}

// GET /download
// @param mediaId - the ID of the ROM to download.
func (server *Server) download(w http.ResponseWriter, r *http.Request) error {
	mediaId := r.URL.Query().Get("mediaId")
	url := "https://download4.vimm.net/download/?mediaId=" + mediaId

	// Create a new request.
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}

	// Add original request headers.
	for k, values := range r.Header {
		if k == "Referer" {
			req.Header.Set("Referer", "https://vimm.net/")
			continue
		}
		for _, v := range values {
			req.Header.Set(k, v)
		}
	}

	// Request content
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to request content. %s", err.Error())
	}

	// Read body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to read body: %s", err.Error())
	}

	defer resp.Body.Close()

	// Save to disk
	path := mediaId + ".zip"
	if err := ioutil.WriteFile(path, body, 0644); err != nil {
		return fmt.Errorf("Failed to write zip: %s", err.Error())
	}

	if err := unzip(path); err != nil {
		return err
	}

	// Remove the zip file
	if err := os.Remove(path); err != nil {
		log.Printf("ERROR removing the temp zip file. %v", err.Error())
	}

	w.Write([]byte("Saved"))
	return nil
}

func withErrorHandling(fn handler) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			log.Printf("ERROR: %v\n", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}
	}
}

func (server *Server) Start() error {
	http.HandleFunc("/download", withErrorHandling(server.download))
	http.HandleFunc("/", server.index)

	address := fmt.Sprintf("%s:%d", server.host, server.port)
	log.Printf("Listening at %s\n", address)
	log.Fatal(http.ListenAndServe(address, nil))

	return nil
}
