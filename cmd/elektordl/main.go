// elektordl downloads Elektor magazine issues from archive.org.
//
// By default it fetches the Hexadoku era (2006 and later) from the
// known archive.org items into -dir, skipping files that are already
// present with the right size. Use -from/-to or -all to fetch other
// years, and -items to add further archive.org items.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type iaFile struct {
	Name string `json:"name"`
	Size string `json:"size"`
}

type iaMetadata struct {
	Files []iaFile `json:"files"`
}

var yearRe = regexp.MustCompile(`(19|20)\d\d`)

func fileYear(name string) int {
	m := yearRe.FindString(name)
	if m == "" {
		return 0
	}
	y, _ := strconv.Atoi(m)
	return y
}

type job struct {
	item, name string
	size       int64
}

func main() {
	dir := flag.String("dir", "elektor_pdfs", "target directory")
	items := flag.String("items", "ElektorMagazine,elektorelectronics20160910",
		"comma-separated archive.org item identifiers")
	from := flag.Int("from", 2006, "first year to fetch")
	to := flag.Int("to", 2100, "last year to fetch")
	all := flag.Bool("all", false, "ignore the year filter and fetch everything")
	dry := flag.Bool("dry", false, "only list what would be downloaded")
	workers := flag.Int("j", 3, "parallel downloads")
	flag.Parse()

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var jobs []job
	var total int64
	for _, item := range strings.Split(*items, ",") {
		item = strings.TrimSpace(item)
		resp, err := http.Get("https://archive.org/metadata/" + url.PathEscape(item))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", item, err)
			os.Exit(1)
		}
		var md iaMetadata
		err = json.NewDecoder(resp.Body).Decode(&md)
		resp.Body.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", item, err)
			os.Exit(1)
		}
		for _, f := range md.Files {
			if !strings.HasSuffix(strings.ToLower(f.Name), ".pdf") ||
				strings.Contains(f.Name, "_text") {
				continue
			}
			if !*all {
				if y := fileYear(f.Name); y < *from || y > *to {
					continue
				}
			}
			size, _ := strconv.ParseInt(f.Size, 10, 64)
			if st, err := os.Stat(filepath.Join(*dir, f.Name)); err == nil && st.Size() == size && size > 0 {
				continue // already complete
			}
			jobs = append(jobs, job{item, f.Name, size})
			total += size
		}
	}

	fmt.Printf("%d files to download, %.1f MB total\n", len(jobs), float64(total)/1e6)
	if *dry {
		for _, j := range jobs {
			fmt.Printf("  %s/%s (%.1f MB)\n", j.item, j.name, float64(j.size)/1e6)
		}
		return
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed int
		done   int
		ch     = make(chan job)
	)
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				err := download(*dir, j)
				mu.Lock()
				if err != nil {
					failed++
					fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", j.name, err)
				} else {
					done++
					fmt.Printf("ok [%d/%d] %s (%.1f MB)\n", done, len(jobs), j.name, float64(j.size)/1e6)
				}
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()
	fmt.Printf("done: %d ok, %d failed\n", done, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func download(dir string, j job) error {
	u := "https://archive.org/download/" + url.PathEscape(j.item) + "/" + url.PathEscape(j.name)
	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	tmp := filepath.Join(dir, j.name+".part")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, j.name))
}
