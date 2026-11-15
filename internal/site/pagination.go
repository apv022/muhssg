package site

import "fmt"

// Paginator splits a list page's entries across several URLs:
// /writing/, /writing/page/2/, /writing/page/3/, …
// Every list and tag page has one; without `paginate` there is a single page
// holding every entry, so templates can always use .Paginator.Items.
type Paginator struct {
	Items      []*Page // the entries on this page
	PageNumber int     // 1-based
	TotalPages int
	PerPage    int
	TotalItems int
	Links      []PageLink // one per page, for numbered navigation

	first string
	urls  []string
}

// PageLink is one entry of Paginator.Links.
type PageLink struct {
	Number  int
	URL     string
	Current bool
}

func (p *Paginator) HasPrev() bool { return p.PageNumber > 1 }
func (p *Paginator) HasNext() bool { return p.PageNumber < p.TotalPages }

// PrevURL is the previous page's URL, or "" on the first page.
func (p *Paginator) PrevURL() string {
	if !p.HasPrev() {
		return ""
	}
	return p.urls[p.PageNumber-2]
}

// NextURL is the next page's URL, or "" on the last page.
func (p *Paginator) NextURL() string {
	if !p.HasNext() {
		return ""
	}
	return p.urls[p.PageNumber]
}

func (p *Paginator) FirstURL() string { return p.urls[0] }
func (p *Paginator) LastURL() string  { return p.urls[len(p.urls)-1] }

// paginate gives base a Paginator and returns the extra pages (2…n) it needs.
func paginate(s *Site, base *Page, items []*Page, perPage int) []*Page {
	if perPage <= 0 || len(items) <= perPage {
		base.Paginator = &Paginator{Items: items, PageNumber: 1, TotalPages: 1, PerPage: len(items), TotalItems: len(items), urls: []string{s.RelURL(base.Path)}}
		base.Paginator.Links = []PageLink{{Number: 1, URL: base.Paginator.urls[0], Current: true}}
		return nil
	}
	total := (len(items) + perPage - 1) / perPage
	paths := make([]string, total)
	urls := make([]string, total)
	for i := range paths {
		if i == 0 {
			paths[i] = base.Path
		} else {
			paths[i] = fmt.Sprintf("%spage/%d/", base.Path, i+1)
		}
		urls[i] = s.RelURL(paths[i])
	}
	var extra []*Page
	for i := 0; i < total; i++ {
		lo, hi := i*perPage, min((i+1)*perPage, len(items))
		pg := &Paginator{Items: items[lo:hi], PageNumber: i + 1, TotalPages: total, PerPage: perPage, TotalItems: len(items), urls: urls}
		for n := range urls {
			pg.Links = append(pg.Links, PageLink{Number: n + 1, URL: urls[n], Current: n == i})
		}
		if i == 0 {
			base.Paginator = pg
			continue
		}
		cp := *base
		cp.Path = paths[i]
		cp.Paginator = pg
		cp.Aliases = nil
		cp.Source = ""
		cp.body, cp.isHTML = nil, false
		extra = append(extra, &cp)
	}
	// /writing/page/1/ is a common guess; send it to the first page.
	s.aliases[base.Path+"page/1/"] = base.Path
	return extra
}
