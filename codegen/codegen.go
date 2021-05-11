package codegen

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"github.com/beevik/etree"
	"github.com/spf13/afero"
	"go/format"
	"os"
	"strings"
	"text/template"
)

var fs = afero.NewOsFs()

//go:embed templates/*
var templates embed.FS

type ProtocolInfo struct {
	ServiceID           string
	ProtocolType        string
	ProtocolVersion     string
	ProtocolDescription string
}

type MessageInfo struct {
	MsgOrder       string
	MsgName        string
	MsgDescription string
	MsgHandler     string
	MsgAccessLvl   string
}

type Field struct {
	Name  string
	Value string
}

// HACK: Fields could be a map[string]string in theory, but the templating engine
// will sort by keys and we need to preserve the order of the fields
type Message struct {
	Type   string
	Info   MessageInfo
	Fields []Field
}

func Generate(messagesPath string) error {
	doc := etree.NewDocument()

	err := doc.ReadFromFile(messagesPath)
	if err != nil {
		return err
	}

	err = printProtocolInfo(doc)
	if err != nil {
		return err
	}

	err = printMessages(doc)
	if err != nil {
		return err
	}

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
		return "int"
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

func printMessages(doc *etree.Document) error {
	tmpl, err := template.ParseFS(templates, "templates/message.tmpl")
	if err != nil {
		return err
	}

	msgs, err := readMessages(doc)
	if err != nil {
		return err
	}

	// Sadly need to buffer in memory because go/format doesn't want an io.Reader
	buf := new(bytes.Buffer)
	writer := bufio.NewWriter(buf)

	for _, msg := range *msgs {
		buf.Reset()

		err = tmpl.Execute(writer, msg)
		if err != nil {
			return err
		}
		writer.Flush()

		fmtd, err := format.Source(buf.Bytes())
		if err != nil {
			return err
		}

		fmt.Print(string(fmtd))
	}

	return nil
}

func printProtocolInfo(doc *etree.Document) error {
	tmpl, err := template.ParseFS(templates, "templates/meta.tmpl")
	if err != nil {
		return err
	}

	meta, err := readProtocolInfo(doc)
	if err != nil {
		return err
	}

	err = tmpl.Execute(os.Stdout, meta)
	if err != nil {
		return err
	}

	return nil
}

func readMessages(doc *etree.Document) (*[]Message, error) {
	records := doc.FindElements("//RECORD")
	if records == nil {
		return nil, ErrorMissingRecords
	}

	var messages []Message

	// Skip first record which is protocol info
	for _, record := range records[1:] {
		fields := record.FindElements("*")
		if fields == nil {
			return nil, ErrorInvalidRecord
		}

		var msg Message

		for _, field := range fields {
			switch field.Tag {
			case "_MsgOrder":
				msg.Info.MsgOrder = field.Text()
			case "_MsgName":
				msg.Info.MsgName = field.Text()
			case "_MsgDescription":
				msg.Info.MsgDescription = field.Text()
			case "_MsgHandler":
				txt := field.Text()

				// HACK: Use the handler name as the message type because it's already PascalCased for us
				msg.Type = strings.ReplaceAll(txt[4:], "_", "")
				msg.Info.MsgHandler = txt
			case "_MsgAccessLvl":
				msg.Info.MsgAccessLvl = field.Text()
			default:
				attr := field.SelectAttr("TYPE")
				if attr == nil {
					return nil, ErrorInvalidMessage
				}
				field := Field{
					Name:  field.Tag,
					Value: dmlToGoType(attr.Value),
				}
				msg.Fields = append(msg.Fields, field)
			}
		}

		messages = append(messages, msg)
	}

	return &messages, nil
}

func readProtocolInfo(doc *etree.Document) (*ProtocolInfo, error) {
	record := doc.FindElement("//_ProtocolInfo/RECORD")
	if record == nil {
		return nil, ErrorMissingProtocolInfo
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
			return nil, ErrorMissingProtocolInfo
		}
		search[k] = val.Text()
	}

	return &ProtocolInfo{
		ServiceID:           search["ServiceID"],
		ProtocolType:        search["ProtocolType"],
		ProtocolVersion:     search["ProtocolVersion"],
		ProtocolDescription: search["ProtocolDescription"],
	}, nil
}
