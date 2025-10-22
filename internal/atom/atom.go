package atom

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

type Feed struct {
	PublicID  string
	Title     string
	Icon      *string
	EmailIcon *string
}

type Entry struct {
	ID         int64
	PublicID   string
	CreatedAt  string
	Author     *string
	Title      string
	Content    string
	Enclosures []Enclosure
}

type Enclosure struct {
	PublicID string
	Type     string
	Length   int64
	Name     string
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Xmlns   string      `xml:"xmlns,attr"`
	ID      string      `xml:"id"`
	Links   []atomLink  `xml:"link"`
	Icon    *string     `xml:"icon,omitempty"`
	Updated string      `xml:"updated"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Rel    string `xml:"rel,attr"`
	Href   string `xml:"href,attr"`
	Type   string `xml:"type,attr,omitempty"`
	Length string `xml:"length,attr,omitempty"`
}

type atomEntry struct {
	ID        string      `xml:"id"`
	Links     []atomLink  `xml:"link"`
	Published string      `xml:"published"`
	Updated   string      `xml:"updated"`
	Author    atomAuthor  `xml:"author"`
	Title     string      `xml:"title"`
	Content   atomContent `xml:"content"`
}

type atomAuthor struct {
	Name  string `xml:"name"`
	Email string `xml:"email"`
}

type atomContent struct {
	Type string `xml:"type,attr"`
	Body string `xml:",innerxml"`
}

// BuildFeedXML returns a full Atom feed document for the given items.
func BuildFeedXML(hostname string, feed Feed, entries []Entry) (string, error) {
	icon := feed.Icon
	if icon == nil && feed.EmailIcon != nil {
		icon = feed.EmailIcon
	}
	af := atomFeed{
		Xmlns: "http://www.w3.org/2005/Atom",
		ID:    fmt.Sprintf("urn:kill-the-newsletter:%s", feed.PublicID),
		Links: []atomLink{
			{Rel: "self", Href: fmt.Sprintf("https://%s/feeds/%s.xml", hostname, feed.PublicID)},
			{Rel: "hub", Href: fmt.Sprintf("https://%s/feeds/%s/websub", hostname, feed.PublicID)},
		},
		Title: feed.Title,
	}
	if icon != nil {
		af.Icon = icon
	}
	if len(entries) > 0 {
		af.Updated = entries[0].CreatedAt
	} else {
		af.Updated = "2000-01-01T00:00:00.000Z"
	}
	for _, e := range entries {
		links := []atomLink{
			{Rel: "alternate", Type: "text/html", Href: fmt.Sprintf("https://%s/feeds/%s/entries/%s.html", hostname, feed.PublicID, e.PublicID)},
		}
		for _, enc := range e.Enclosures {
			links = append(links, atomLink{Rel: "enclosure", Type: enc.Type, Length: fmt.Sprintf("%d", enc.Length), Href: fmt.Sprintf("https://%s/files/%s/%s", hostname, enc.PublicID, enc.Name)})
		}
		ae := atomEntry{
			ID:        fmt.Sprintf("urn:kill-the-newsletter:%s", e.PublicID),
			Links:     links,
			Published: e.CreatedAt,
			Updated:   e.CreatedAt,
			Author:    atomAuthor{Name: valOr(e.Author, "Kill the Newsletter!"), Email: valOr(e.Author, "kill-the-newsletter@leafac.com")},
			Title:     e.Title,
			Content:   atomContent{Type: "html", Body: fmt.Sprintf("%s<hr /><p><small><a href=\"https://%s/feeds/%s\">Kill the Newsletter! feed settings</a></small></p>", e.Content, hostname, feed.PublicID)},
		}
		af.Entries = append(af.Entries, ae)
	}
	buf := &bytes.Buffer{}
	buf.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n")
	enc := xml.NewEncoder(buf)
	enc.Indent("", "  ")
	if err := enc.Encode(af); err != nil {
		return "", err
	}
	if err := enc.Flush(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func valOr(p *string, def string) string {
	if p != nil && *p != "" {
		return *p
	}
	return def
}
