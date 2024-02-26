// Translate original dump files into new JSON format.
package main

import (
	"encoding/json"
	"github.com/anaminus/but"
	"github.com/robloxapi/rbxapi"
	"github.com/robloxapi/rbxapi/diff"
	"github.com/robloxapi/rbxapi/rbxapidump"
	"github.com/robloxapi/rbxapi/rbxapijson"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	StablePath  = `../stable.txt`
	JStablePath = `../stable.json`
	BuildsPath  = `../../builds.json`
	InputPath   = `../../data/api-dump/txt`
	OutputPath  = `../../data/api-dump/json`
)

type Data struct {
	Root *rbxapijson.Root
	Next *Data
}

// Visit each type in the API.
func VisitTypes(root *rbxapijson.Root, visit func(rbxapi.Type)) {
	for _, class := range root.GetClasses() {
		visit(rbxapijson.Type{Category: "Class", Name: class.GetName()})
		visit(rbxapijson.Type{Category: "Class", Name: class.GetSuperclass()})
		for _, member := range class.GetMembers() {
			switch member.GetMemberType() {
			case "Property":
				member := member.(rbxapi.Property)
				visit(member.GetValueType())
			case "Function":
				member := member.(rbxapi.Function)
				for _, param := range member.GetParameters().GetParameters() {
					visit(param.GetType())
				}
				visit(member.GetReturnType())
			case "Event":
				member := member.(rbxapi.Event)
				for _, param := range member.GetParameters().GetParameters() {
					visit(param.GetType())
				}
			case "Callback":
				member := member.(rbxapi.Callback)
				for _, param := range member.GetParameters().GetParameters() {
					visit(param.GetType())
				}
				visit(member.GetReturnType())
			}
		}
	}
	for _, enum := range root.GetEnums() {
		visit(rbxapijson.Type{Category: "Enum", Name: enum.GetName()})
	}
}

// Visit each class in the API.
func VisitClasses(root rbxapi.Root, visit func(rbxapi.Class)) {
	for _, class := range root.GetClasses() {
		visit(class)
	}
}

// Visit each member of each class in the API.
func VisitMembers(root rbxapi.Root, visit func(rbxapi.Class, rbxapi.Member)) {
	for _, class := range root.GetClasses() {
		for _, member := range class.GetMembers() {
			visit(class, member)
		}
	}
}

// Visit each enum in the API.
func VisitEnums(root rbxapi.Root, visit func(rbxapi.Enum)) {
	for _, enum := range root.GetEnums() {
		visit(enum)
	}
}

// Visit each item of each enum in the API.
func VisitEnumItems(root rbxapi.Root, visit func(rbxapi.Enum, rbxapi.EnumItem)) {
	for _, enum := range root.GetEnums() {
		for _, item := range enum.GetEnumItems() {
			visit(enum, item)
		}
	}
}

func FindEntity(data *Data, primary, secondary interface{}) interface{} {
	switch primary := primary.(type) {
	case rbxapi.Class:
		class := data.Root.GetClass(primary.GetName())
		if class == nil {
			goto finish
		}
		switch secondary := secondary.(type) {
		case rbxapi.Member:
			member := class.GetMember(secondary.GetName())
			if member == nil {
				goto finish
			}
			return member
		case nil:
			return class
		}
	case rbxapi.Enum:
		enum := data.Root.GetEnum(primary.GetName())
		if enum == nil {
			goto finish
		}
		switch secondary := secondary.(type) {
		case rbxapi.EnumItem:
			item := enum.GetEnumItem(secondary.GetName())
			if item == nil {
				goto finish
			}
			return item
		case nil:
			return enum
		}
	}
finish:
	if data.Next != nil {
		return FindEntity(data.Next, primary, secondary)
	}
	return nil
}

// Rewrite history to resolve naming conflicts.
func ResolveConflicts(root *rbxapidump.Root) {
	foundPages := false
	VisitClasses(root, func(c rbxapi.Class) {
		class := c.(*rbxapidump.Class)
		// Second instance of Pages class. Was immediately renamed to
		// StandardPages in the next version.
		switch class.Name {
		case "Pages":
			if foundPages {
				class.Name = "StandardPages"
			} else {
				foundPages = true
			}
		case "DataModel":
			// Many versions saw a DataModel.Loaded function, which conflicted
			// with the Loaded event. Apparently it went unused, and so was
			// ultimately removed. The same is done here. Although it might be
			// worth renaming it instead, there isn't any specific name that can
			// be used.
			members := class.Members[:0]
			for _, member := range class.Members {
				if member.GetName() == "Loaded" &&
					member.GetMemberType() == "Function" {
					continue
				}
				members = append(members, member)
			}
			for i := len(members); i < len(class.Members); i++ {
				class.Members[i] = nil
			}
			class.Members = members
		}
	})
	foundCameraMode := false
	VisitEnums(root, func(e rbxapi.Enum) {
		enum := e.(*rbxapidump.Enum)
		// Second instance of CameraMode enum. Was renamed to CustomCameraMode
		// after several versions.
		switch enum.Name {
		case "CameraMode":
			if foundCameraMode {
				enum.Name = "CustomCameraMode"
			} else {
				foundCameraMode = true
			}
		case "KeyCode":
			// Many versions saw a number of redundant KeyCode.KeypadEquals enum
			// items. All the extras were eventually removed, so they're not
			// very interesting for keeping around.
			foundKeypadEquals := false
			items := enum.Items[:0]
			for _, item := range enum.Items {
				if item.Name == "KeypadEquals" {
					if foundKeypadEquals {
						continue
					} else {
						foundKeypadEquals = true
					}
				}
				items = append(items, item)
			}
			for i := len(items); i < len(enum.Items); i++ {
				enum.Items[i] = nil
			}
			enum.Items = items
		}
	})
	foundRunning := false
	VisitEnumItems(root, func(e rbxapi.Enum, i rbxapi.EnumItem) {
		enum := e.(*rbxapidump.Enum)
		item := i.(*rbxapidump.EnumItem)
		// Second instance of Running enum item. Was renamed to RunningNoPhysics
		// after many versions.
		if enum.Name == "HumanoidStateType" && item.Name == "Running" {
			if foundRunning {
				item.Name = "RunningNoPhysics"
			} else {
				foundRunning = true
			}
		}
	})
}

// func FindRenamedTypes(root *rbxapidump.Root, jroot *rbxapijson.Root) map[string]rbxapijson.Type {
// 	types := map[string]rbxapijson.Type{}
// 	checkType := func(a, b rbxapi.Type) {
// 		if a.GetName() != b.GetName() {
// 			types[a.GetName()] = rbxapijson.Type{
// 				Category: b.GetCategory(),
// 				Name:     b.GetName(),
// 			}
// 		}
// 	}
// 	type Params interface {
// 		GetParameters() rbxapi.Parameters
// 	}
// 	checkParameters := func(a, b Params) bool {
// 		aps := a.GetParameters().GetParameters()
// 		bps := b.GetParameters().GetParameters()
// 		if len(aps) != len(bps) {
// 			return false
// 		}
// 		for _, ap := range aps {
// 			for _, bp := range bps {
// 				if ap.GetName() != bp.GetName() {
// 					return false
// 				}
// 			}
// 		}
// 		for i, ap := range aps {
// 			checkType(ap.GetType(), bps[i].GetType())
// 		}
// 		return true
// 	}
// 	VisitMembers(root, func(class rbxapi.Class, member rbxapi.Member) {
// 		switch member.GetMemberType() {
// 		case "Property":
// 			a, _ := member.(rbxapi.Property)
// 			b, _ := FindEntity(jroot, class, member).(rbxapi.Property)
// 			if a == nil || b == nil {
// 				return
// 			}
// 			checkType(a.GetValueType(), b.GetValueType())
// 		case "Function":
// 			a, _ := member.(rbxapi.Function)
// 			b, _ := FindEntity(jroot, class, member).(rbxapi.Function)
// 			if a == nil || b == nil {
// 				return
// 			}
// 			checkType(a.GetReturnType(), b.GetReturnType())
// 			if !checkParameters(a, b) {
// 				but.Logf("parameters of %s.%s do not match\n", class.GetName(), member.GetName())
// 				return
// 			}
// 		case "Event":
// 			a, _ := member.(rbxapi.Event)
// 			b, _ := FindEntity(jroot, class, member).(rbxapi.Event)
// 			if a == nil || b == nil {
// 				return
// 			}
// 			if !checkParameters(a, b) {
// 				but.Logf("parameters of %s.%s do not match\n", class.GetName(), member.GetName())
// 				return
// 			}
// 		case "Callback":
// 			a, _ := member.(rbxapi.Callback)
// 			b, _ := FindEntity(jroot, class, member).(rbxapi.Callback)
// 			if a == nil || b == nil {
// 				return
// 			}
// 			checkType(a.GetReturnType(), b.GetReturnType())
// 			if !checkParameters(a, b) {
// 				but.Logf("parameters of %s.%s do not match\n", class.GetName(), member.GetName())
// 				return
// 			}
// 		}
// 	})
// 	return types
// }

// Correct errors in translation of the current root, using the root of the next
// build as a reference.
func CorrectErrors(data *Data, correctors []interface{}) {
	type RootCorrector interface {
		Root(current, next *rbxapijson.Root)
	}
	type ClassCorrector interface {
		Class(current, next *rbxapijson.Class)
	}
	type PropertyCorrector interface {
		Property(current, next *rbxapijson.Property)
	}
	type FunctionCorrector interface {
		Function(current, next *rbxapijson.Function)
	}
	type EventCorrector interface {
		Event(current, next *rbxapijson.Event)
	}
	type CallbackCorrector interface {
		Callback(current, next *rbxapijson.Callback)
	}
	type EnumCorrector interface {
		Enum(current, next *rbxapijson.Enum)
	}
	type EnumItemCorrector interface {
		EnumItem(current, next *rbxapijson.EnumItem)
	}

	for _, corrector := range correctors {
		if rootCorrector, ok := corrector.(RootCorrector); ok {
			if data.Next != nil {
				rootCorrector.Root(data.Root, data.Next.Root)
			} else {
				rootCorrector.Root(data.Root, nil)
			}
		}
		if classCorrector, ok := corrector.(ClassCorrector); ok {
			VisitClasses(data.Root, func(c rbxapi.Class) {
				class := c.(*rbxapijson.Class)
				nclass, _ := FindEntity(data.Next, class, nil).(*rbxapijson.Class)
				classCorrector.Class(class, nclass)
			})
		}
		VisitMembers(data.Root, func(c rbxapi.Class, m rbxapi.Member) {
			class := c.(*rbxapijson.Class)
			switch member := m.(type) {
			case *rbxapijson.Property:
				if propertyCorrector, ok := corrector.(PropertyCorrector); ok {
					nmember, _ := FindEntity(data.Next, class, member).(*rbxapijson.Property)
					propertyCorrector.Property(member, nmember)
				}
			case *rbxapijson.Function:
				if functionCorrector, ok := corrector.(FunctionCorrector); ok {
					nmember, _ := FindEntity(data.Next, class, member).(*rbxapijson.Function)
					functionCorrector.Function(member, nmember)
				}
			case *rbxapijson.Event:
				if eventCorrector, ok := corrector.(EventCorrector); ok {
					nmember, _ := FindEntity(data.Next, class, member).(*rbxapijson.Event)
					eventCorrector.Event(member, nmember)
				}
			case *rbxapijson.Callback:
				if callbackCorrector, ok := corrector.(CallbackCorrector); ok {
					nmember, _ := FindEntity(data.Next, class, member).(*rbxapijson.Callback)
					callbackCorrector.Callback(member, nmember)
				}
			}
		})
		if enumCorrector, ok := corrector.(EnumCorrector); ok {
			VisitEnums(data.Root, func(e rbxapi.Enum) {
				enum := e.(*rbxapijson.Enum)
				nenum, _ := FindEntity(data.Next, enum, nil).(*rbxapijson.Enum)
				enumCorrector.Enum(enum, nenum)
			})
		}
		if enumItemCorrector, ok := corrector.(EnumItemCorrector); ok {
			VisitEnumItems(data.Root, func(e rbxapi.Enum, i rbxapi.EnumItem) {
				enum := e.(*rbxapijson.Enum)
				item := i.(*rbxapijson.EnumItem)
				nitem, _ := FindEntity(data.Next, enum, item).(*rbxapijson.EnumItem)
				enumItemCorrector.EnumItem(item, nitem)
			})
		}
	}
}

type Types map[string][]rbxapijson.Type

func (types Types) TransformType(dst *rbxapijson.Type, src *rbxapijson.Type) {
	// Try getting category from source.
	if src != nil {
		if dst.Category == "" && src.Name == dst.Name {
			dst.Category = src.Category
		}
	}
	// Try getting category from corpus of known types.
	if dst.Category == "" {
		if ts := types[dst.Name]; len(ts) > 0 {
			dst.Category = ts[0].Category
			// If there were more than one type mapped to the name, then we
			// would have to figure out which to use based on the context.
			// Thankfully, no conflicting type names were used in the dump.
		}
	}
	// Rename types that were changed in stable.
	if src != nil {
		// TODO: Renaming these types could be considered a legitimate change.
		// Conversely, the names are unchanged in the dump format of the same
		// version as stable, so they could be considered format-specific.
		switch dst.Name {
		case "EventInstance",
			"Property",
			"CoordinateFrame",
			"Connection",
			"Rect2D":
			dst.Name = src.Name
			// Try getting category from new name, if necessary.
			if dst.Category == "" {
				if ts := types[dst.Name]; len(ts) > 0 {
					dst.Category = ts[0].Category
				}
			}
		}
	}
}

func (types Types) TransformParameters(dst, src *[]rbxapijson.Parameter) {
	if src == nil {
		for i := range *dst {
			types.TransformType(&((*dst)[i].Type), nil)
		}
		return
	}
	srcp := *src
	unvisited := make(map[int]struct{}, len(*dst))
	for i := range *dst {
		unvisited[i] = struct{}{}
	}
	for i, p := range *dst {
		for j, fp := range srcp {
			switch {
			case
				fp.Name == p.Name,
				fp.Name != p.Name && i == j:
				types.TransformType(&((*dst)[i].Type), &fp.Type)
				delete(unvisited, i)
				break
			}
		}
	}
	for i := range unvisited {
		types.TransformType(&((*dst)[i].Type), nil)
	}
}

func (types Types) Visit(typ rbxapi.Type) {
	cat := typ.GetCategory()
	if cat == "" {
		return
	}
	name := typ.GetName()
	for _, t := range types[name] {
		if t.GetCategory() == cat {
			return
		}
	}
	types[name] = append(types[name], rbxapijson.Type{Category: typ.GetCategory(), Name: typ.GetName()})
}

type CorrectEnums struct {
	Enums map[string]struct{}
}

func (c CorrectEnums) Root(current, next *rbxapijson.Root) {
	// Backport missing enum list.
	if len(current.Enums) == 0 {
		current.Enums = make([]*rbxapijson.Enum, 0, len(next.Enums))
		for _, enum := range next.Enums {
			if _, ok := c.Enums[enum.Name]; ok {
				current.Enums = append(current.Enums, enum.Copy().(*rbxapijson.Enum))
			}
		}
	}
}

type CorrectTypes struct {
	Types Types
}

func (c CorrectTypes) Property(current, next *rbxapijson.Property) {
	if next != nil {
		c.Types.TransformType(&current.ValueType, &next.ValueType)
	} else {
		c.Types.TransformType(&current.ValueType, nil)
	}
}
func (c CorrectTypes) Function(current, next *rbxapijson.Function) {
	if next != nil {
		c.Types.TransformParameters(&current.Parameters, &next.Parameters)
		c.Types.TransformType(&current.ReturnType, &next.ReturnType)
	} else {
		c.Types.TransformParameters(&current.Parameters, nil)
		c.Types.TransformType(&current.ReturnType, nil)
	}
}
func (c CorrectTypes) Event(current, next *rbxapijson.Event) {
	if next != nil {
		c.Types.TransformParameters(&current.Parameters, &next.Parameters)
	} else {
		c.Types.TransformParameters(&current.Parameters, nil)
	}
}
func (c CorrectTypes) Callback(current, next *rbxapijson.Callback) {
	if next != nil {
		c.Types.TransformParameters(&current.Parameters, &next.Parameters)
		c.Types.TransformType(&current.ReturnType, &next.ReturnType)
	} else {
		c.Types.TransformParameters(&current.Parameters, nil)
		c.Types.TransformType(&current.ReturnType, nil)
	}
}

type CorrectFields struct{}

func (c CorrectFields) Class(current, next *rbxapijson.Class) {
	if next != nil {
		if current.Superclass == "" {
			current.Superclass = next.Superclass
		}
		current.MemoryCategory = next.MemoryCategory
	} else {
		if current.Superclass == "" && current.Name != "<<<ROOT>>>" {
			// This applies to instances that where removed before superclasses
			// were exposed in the dump. Besides ROOT, which doesn't have a
			// superclass, the only other class that meets this condition is
			// PseudoPlayer, which is presumed to have inherited from Instance.
			current.Superclass = "Instance"
		}
	}
}
func (c CorrectFields) Property(current, next *rbxapijson.Property) {
	if next != nil {
		current.Category = next.Category
		current.CanLoad = next.CanLoad
		current.CanSave = next.CanSave
	}
}

type CorrectTags struct{}

func (c CorrectTags) correctSecurity(security *string, tags *rbxapijson.Tags) {
	for _, tag := range tags.GetTags() {
		switch {
		case strings.Contains(tag, "Security"),
			strings.Contains(tag, "security"):
			*security = tag
			tags.UnsetTag(tag)
		}
	}
	if *security == "" {
		*security = "None"
	}
}
func (c CorrectTags) overwriteTags(dst, src *rbxapijson.Tags) {
	*dst = (*dst)[:0]
	dst.SetTag(src.GetTags()...)
}
func (c CorrectTags) renameTag(tags *rbxapijson.Tags, from, to string) {
	if tags.GetTag(from) {
		tags.UnsetTag(from)
		tags.SetTag(to)
	}
}
func (c CorrectTags) Class(current, next *rbxapijson.Class) {
	c.renameTag(&current.Tags, "notCreatable", "NotCreatable")
	if next != nil {
		if next.GetTag("NotCreatable") {
			current.SetTag("NotCreatable")
		}
		if next.GetTag("Service") {
			current.SetTag("Service")
		}
		if next.GetTag("NotReplicated") {
			current.SetTag("NotReplicated")
		}
		if next.GetTag("PlayerReplicated") {
			current.SetTag("PlayerReplicated")
		}
	}
	c.renameTag(&current.Tags, "notbrowsable", "NotBrowsable")
	c.renameTag(&current.Tags, "deprecated", "Deprecated")

	if current.Name == "Instance" {
		if member, _ := current.GetMember("WaitForChild").(*rbxapijson.Function); member != nil {
			if !member.GetTag("Yields") {
				// Backport CanYield tag back to the point where WaitForChild
				// was a YieldFunction.
				member.SetTag("CanYield")
			}
		}
	}
}
func (c CorrectTags) Property(current, next *rbxapijson.Property) {
	for _, tag := range current.GetTags() {
		const prefix = "ScriptWriteRestricted: ["
		switch {
		case strings.HasPrefix(tag, prefix):
			current.WriteSecurity = tag[len(prefix) : len(tag)-1]
			current.UnsetTag(tag)
		case strings.Contains(tag, "Security"),
			strings.Contains(tag, "security"):
			current.ReadSecurity = tag
			current.WriteSecurity = tag
			current.UnsetTag(tag)
		}
	}
	if current.WriteSecurity == "" {
		current.WriteSecurity = "None"
	}
	if current.ReadSecurity == "" {
		current.ReadSecurity = "None"
	}
	c.renameTag(&current.Tags, "hidden", "Hidden")
	c.renameTag(&current.Tags, "readonly", "ReadOnly")
	if next != nil {
		if next.GetTag("NotReplicated") {
			current.SetTag("NotReplicated")
		}
	}
	c.renameTag(&current.Tags, "notbrowsable", "NotBrowsable")
	c.renameTag(&current.Tags, "deprecated", "Deprecated")
}
func (c CorrectTags) Function(current, next *rbxapijson.Function) {
	c.correctSecurity(&current.Security, &current.Tags)
	c.renameTag(&current.Tags, "notbrowsable", "NotBrowsable")
	c.renameTag(&current.Tags, "deprecated", "Deprecated")
}
func (c CorrectTags) Event(current, next *rbxapijson.Event) {
	c.correctSecurity(&current.Security, &current.Tags)
	c.renameTag(&current.Tags, "notbrowsable", "NotBrowsable")
	c.renameTag(&current.Tags, "deprecated", "Deprecated")
}
func (c CorrectTags) Callback(current, next *rbxapijson.Callback) {
	c.correctSecurity(&current.Security, &current.Tags)
	c.renameTag(&current.Tags, "notbrowsable", "NotBrowsable")
	c.renameTag(&current.Tags, "deprecated", "Deprecated")
}
func (c CorrectTags) Enum(current, next *rbxapijson.Enum) {
	c.renameTag(&current.Tags, "notbrowsable", "NotBrowsable")
	c.renameTag(&current.Tags, "deprecated", "Deprecated")
}
func (c CorrectTags) EnumItem(current, next *rbxapijson.EnumItem) {
	c.renameTag(&current.Tags, "notbrowsable", "NotBrowsable")
	c.renameTag(&current.Tags, "deprecated", "Deprecated")
}

type Build struct {
	Hash    string
	Date    time.Time
	Version string
}

func main() {
	var err error
	_ = err

	// var stable *rbxapidump.Root
	// {
	// 	f, err := os.Open(StablePath)
	// 	but.IfFatal(err, "open stable dump")
	// 	stable, err = rbxapidump.Decode(f)
	// 	f.Close()
	// 	but.IfFatal(err, "decode stable dump")
	// }
	var jstable *rbxapijson.Root
	{
		f, err := os.Open(JStablePath)
		but.IfFatal(err, "open stable JSON dump")
		jstable, err = rbxapijson.Decode(f)
		f.Close()
		but.IfFatal(err, "decode stable JSON dump")
	}

	// renamedTypes := FindRenamedTypes(stable, jstable)

	// Manually add types that had been removed at some point.
	types := Types{
		// Replaced by "Class:<class>".
		"Object": {{Category: "DataType", Name: "Object"}},
		// Renamed to "CFrame".
		"CoordinateFrame": {{Category: "DataType", Name: "CoordinateFrame"}},
		// All references removed.
		"SystemAddress":        {{Category: "DataType", Name: "SystemAddress"}},
		"BuildPermission":      {{Category: "DataType", Name: "BuildPermission"}},
		"PhysicsReceiveMethod": {{Category: "Enum", Name: "PhysicsReceiveMethod"}},
		"PhysicsSendMethod":    {{Category: "Enum", Name: "PhysicsSendMethod"}},
		"PrismSides":           {{Category: "Enum", Name: "PrismSides"}},
		"PyramidSides":         {{Category: "Enum", Name: "PyramidSides"}},
	}
	VisitTypes(jstable, types.Visit)

	var builds []Build
	{
		f, err := os.Open(BuildsPath)
		but.IfFatal(err, "open builds file")
		err = json.NewDecoder(f).Decode(&builds)
		f.Close()
		but.IfFatal(err, "parse builds file")
	}

	but.IfFatal(os.MkdirAll(OutputPath, 0666), "make output directory")

	sort.Slice(builds, func(i, j int) bool {
		return builds[i].Date.After(builds[j].Date)
	})
	next := &Data{Root: jstable}
	for i, build := range builds {
		_ = i
		// but.Logf("Process %03d/%03d: %s\n", i, len(builds), build.Hash)
		var root *rbxapidump.Root
		{
			f, err := os.Open(filepath.Join(InputPath, build.Hash+".txt"))
			but.IfFatalf(err, "open dump %s", build.Hash)
			root, err = rbxapidump.Decode(f)
			f.Close()
			but.IfFatal(err, "decode dump %s", build.Hash)
		}

		ResolveConflicts(root)
		jroot := &rbxapijson.Root{}
		jroot.Patch((&diff.Diff{Prev: &rbxapidump.Root{}, Next: root}).Diff())
		data := &Data{Root: jroot, Next: next}

		VisitTypes(jroot, types.Visit)

		CorrectErrors(data, []interface{}{
			CorrectTypes{Types: types},
			CorrectFields{},
			CorrectTags{},
		})
		enumTypes := map[string]struct{}{}
		VisitTypes(jroot, func(typ rbxapi.Type) {
			if typ.GetCategory() == "Enum" {
				enumTypes[typ.GetName()] = struct{}{}
			}
		})
		CorrectErrors(data, []interface{}{CorrectEnums{enumTypes}})

		{
			f, err := os.Create(filepath.Join(OutputPath, build.Hash+".json"))
			but.IfFatalf(err, "create output file %s", build.Hash)
			err = rbxapijson.Encode(f, jroot)
			f.Close()
			but.IfFatalf(err, "encode output %s", build.Hash)
		}
		// Current root is now pristine; it can be used as a reference for
		// correction.
		next = data
	}
}
