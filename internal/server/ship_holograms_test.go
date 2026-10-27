package server

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestShipHolograms(t *testing.T) {
	silhouettes := map[string]string{}
	for _, class := range shipClasses {
		t.Run(class.ID, func(t *testing.T) {
			data, err := assets.ReadFile("web/" + class.Model)
			if err != nil {
				t.Fatal(err)
			}
			decoder := xml.NewDecoder(bytes.NewReader(data))
			fills := []string{"black"}
			ids := map[string]bool{}
			refs := []string{}
			orbits, pulses, uses := 0, 0, 0
			shape := ""
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				switch el := token.(type) {
				case xml.StartElement:
					attrs := map[string]string{}
					for _, a := range el.Attr {
						attrs[a.Name.Local] = a.Value
						if strings.HasPrefix(strings.ToLower(a.Name.Local), "on") {
							t.Fatal("script handler in hologram")
						}
					}
					fill := fills[len(fills)-1]
					if v, ok := attrs["fill"]; ok {
						fill = v
					}
					fills = append(fills, fill)
					if id := attrs["id"]; id != "" {
						if ids[id] {
							t.Fatal("duplicate SVG id", id)
						}
						ids[id] = true
					}
					switch el.Name.Local {
					case "script", "foreignObject", "image", "text":
						t.Fatal("hologram must contain only vector geometry", el.Name.Local)
					case "path", "line", "polyline", "polygon", "rect", "circle", "ellipse":
						if fill != "none" {
							t.Fatal("filled shape/background in hologram", el.Name.Local)
						}
						if shape == "" && el.Name.Local == "path" {
							shape = attrs["d"]
						}
					case "use":
						uses++
						ref := attrs["href"]
						if !strings.HasPrefix(ref, "#") {
							t.Fatal("external hologram dependency", ref)
						}
						refs = append(refs, strings.TrimPrefix(ref, "#"))
					case "animate":
						if attrs["repeatCount"] != "indefinite" {
							t.Fatal("animation must loop")
						}
						switch attrs["attributeName"] {
						case "d":
							orbits++
							if attrs["dur"] != "16s" {
								t.Fatal("orbit period must be 16 seconds")
							}
						case "stroke-dashoffset":
							pulses++
						}
					}
				case xml.EndElement:
					fills = fills[:len(fills)-1]
				}
			}
			if orbits < 1 || pulses < 1 || uses < 1 || shape == "" {
				t.Fatal("missing rotation, light pulses or wireframe")
			}
			for _, ref := range refs {
				if !ids[ref] {
					t.Fatal("missing referenced geometry", ref)
				}
			}
			if other := silhouettes[shape]; other != "" {
				t.Fatal("same hull geometry as", other)
			}
			silhouettes[shape] = class.ID
		})
	}
}
