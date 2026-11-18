package tools

import (
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/tasks"
	"github.com/scrothers/aim/internal/wire"
)

// ListState selects deliveries by state in list. Its values are the mail
// package's list states; see enumSchemas.
type ListState string

// MarkState is the state mark moves deliveries to. Its values are the mail
// package's mark states; see enumSchemas.
type MarkState string

// enumSchemas gives each enumerated string type in tool inputs and outputs a
// schema listing its values, so clients and models see the allowed values
// and the server rejects others before a handler runs. Values come from the
// packages that define them.
var enumSchemas = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[ListState]():         enum(mail.StateUnread, mail.StateInbox, mail.StateRead, mail.StateArchived, mail.StateAll),
	reflect.TypeFor[MarkState]():         enum(mail.MarkRead, mail.MarkUnread, mail.MarkArchived, mail.MarkUnarchived),
	reflect.TypeFor[mail.DeliveryKind](): enum(string(mail.DeliveryTo), string(mail.DeliveryCc)),
	reflect.TypeFor[mail.SessionState](): enum(presence.StateOnline, presence.StateIdle),
	reflect.TypeFor[mail.Attention]():    enum(wire.AttentionBlocked, wire.AttentionWaiting, wire.AttentionFailed, wire.AttentionInterrupted),
	reflect.TypeFor[tasks.Status]():      enumOf(tasks.Statuses),
	reflect.TypeFor[tasks.Priority]():    enumOf(tasks.Priorities),
	reflect.TypeFor[tasks.Action]():      enumOf(tasks.Actions),
	reflect.TypeFor[tasks.Health]():      enumOf(tasks.Healths),
}

// enumOf returns a string schema restricted to the values of a named string
// type.
func enumOf[T ~string](values []T) *jsonschema.Schema {
	s := &jsonschema.Schema{Type: "string"}
	for _, v := range values {
		s.Enum = append(s.Enum, string(v))
	}
	return s
}

// enum returns a string schema restricted to values.
func enum(values ...string) *jsonschema.Schema {
	s := &jsonschema.Schema{Type: "string"}
	for _, v := range values {
		s.Enum = append(s.Enum, v)
	}
	return s
}
