package codegen

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"github.com/beevik/etree"
	"github.com/pkg/errors"
	"go/format"
	"io"
	"os"
	"strings"
	"text/template"
)

//go:embed templates/*
var templates embed.FS

type Field struct {
	Name  string
	Value string
}

// HACK: Fields could be a map[string]string in theory, but the templating engine
// will sort by keys and we need to preserve the order of the fields
type Message struct {
	Type string
	Meta struct {
		MsgOrder       string
		MsgName        string
		MsgDescription string
		MsgHandler     string
		MsgAccessLvl   string
	}
	Fields []Field
}

type Protocol struct {
	Package string
	Service string
	Meta    struct {
		ServiceID   string
		Type        string
		Version     string
		Description string
	}
	Messages []Message
}

func Generate(w io.Writer, messagesPath string) error {
	doc := etree.NewDocument()

	err := doc.ReadFromFile(messagesPath)
	if err != nil {
		return err
	}

	var p Protocol
	err = readProtocol(doc, &p)
	if err != nil {
		return err
	}

	tmpl, err := template.ParseFS(templates, "templates/protocol.tmpl")
	if err != nil {
		panic("failed to parse template")
	}

	// Sadly need to buffer in memory because go/format doesn't want an io.Reader
	buf := new(bytes.Buffer)
	writer := bufio.NewWriter(buf)

	err = tmpl.Execute(writer, p)
	if err != nil {
		return err
	}
	writer.Flush()

	fmtd, err := format.Source(buf.Bytes())
	if err != nil {
		return errors.Wrap(err, "generated code had invalid syntax")
	}

	_, err = w.Write(fmtd)
	if err != nil {
		return err
	}

	return nil
}

func readProtocol(doc *etree.Document, p *Protocol) error {
	err := readPackage(doc, p)
	if err != nil {
		return err
	}

	err = readProtocolInfo(doc, p)
	if err != nil {
		return err
	}

	err = readMessages(doc, p)
	if err != nil {
		return err
	}

	return nil
}

func readPackage(doc *etree.Document, p *Protocol) error {
	p.Package = os.Getenv("GOPACKAGE")
	if p.Package == "" {
		return ErrorNoGoPackageEnv
	}

	return nil
}

func readMessages(doc *etree.Document, p *Protocol) error {
	records := doc.FindElements("//RECORD")
	if records == nil {
		return ErrorMissingRecords
	}

	// Skip first record which is protocol info
	for _, record := range records[1:] {
		fields := record.FindElements("*")
		if fields == nil {
			return ErrorInvalidRecord
		}

		var msg Message

		for _, field := range fields {
			switch field.Tag {
			case "_MsgOrder":
				msg.Meta.MsgOrder = field.Text()
			case "_MsgName":
				msg.Meta.MsgName = field.Text()
			case "_MsgDescription":
				msg.Meta.MsgDescription = field.Text()
			case "_MsgHandler":
				txt := field.Text()

				// HACK: Use the handler name as the message type because it's already PascalCased for us
				msg.Type = strings.ReplaceAll(txt[4:], "_", "")
				msg.Meta.MsgHandler = txt
			case "_MsgAccessLvl":
				msg.Meta.MsgAccessLvl = field.Text()
			default:
				attr := field.SelectAttr("TYPE")
				if attr == nil {
					return ErrorInvalidMessage
				}
				field := Field{
					Name:  field.Tag,
					Value: dmlToGoType(attr.Value),
				}
				msg.Fields = append(msg.Fields, field)
			}
		}

		p.Messages = append(p.Messages, msg)
	}

	return nil
}

func readProtocolInfo(doc *etree.Document, p *Protocol) error {
	record := doc.FindElement("//_ProtocolInfo/RECORD")
	if record == nil {
		return ErrorMissingProtocolInfo
	}

	search := map[string]string{
		"ServiceID":           "",
		"ProtocolType":        "",
		"ProtocolVersion":     "",
		"ProtocolDescription": "",
	}
	for k := range search {
		val := record.FindElement(k)
		if val == nil {
			return ErrorMissingProtocolInfo
		}
		search[k] = val.Text()
	}

	p.Meta.ServiceID = search["ServiceID"]
	p.Meta.Type = search["ProtocolType"]
	p.Meta.Version = search["ProtocolVersion"]
	p.Meta.Description = search["ProtocolDescription"]

	p.Service = strings.Title(strings.ToLower(p.Meta.Type))

	return nil
}

func dmlToGoType(typ string) string {
	switch typ {
	case "BYT":
		return "int8"
	case "UBYT":
		return "uint8"
	case "SHRT":
		return "int16"
	case "USHRT":
		return "uint16"
	case "INT":
		return "int32"
	case "UINT":
		return "uint32"
	case "STR":
		return "string"
	case "WSTR":
		// TODO: Think about this
		return "string"
	case "FLT":
		return "float32"
	case "DBL":
		return "float64"
	case "GID":
		return "uint64"
	default:
		panic(fmt.Sprintf("unknown DML type %v", typ))
	}
}
