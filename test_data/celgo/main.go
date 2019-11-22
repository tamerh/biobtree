package main

import (
	"celgo/pbuf"
	"fmt"
	"log"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	typescelgo "github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/interpreter/functions"
	"github.com/pquerna/ffjson/ffjson"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	// typescelgo "github.com/google/cel-go/common/types"
)

func main() {

	x := pbuf.Xref{}
	x.Attributes = &pbuf.Xref_Gattr{&pbuf.GoAttr{Type: "tamer"}}
	x.Attributes = &pbuf.Xref_Uattr{&pbuf.UniAttr{Name: "tamer2"}}

	//b, _ := json.Marshal(x)
	b, _ := ffjson.Marshal(x.Attributes)

	attr := &pbuf.Xref_Uattr{}

	var c interface{}

	c = attr

	err := ffjson.Unmarshal(b, c)
	if err != nil {
		panic(err)
	}

	env, err := cel.NewEnv(
		cel.Types(&pbuf.GoAttr{Type: "tamer"}),
		cel.Types(&pbuf.UniAttr{Name: "tamer"}),
		cel.Declarations(
			decls.NewIdent("g", decls.NewObjectType("pbuf.GoAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("u", decls.NewObjectType("pbuf.UniAttr"), nil)),
		cel.Declarations(
			decls.NewFunction("overlaps",
				decls.NewOverload("overlaps_int_int",
					[]*exprpb.Type{decls.Int, decls.Int},
					decls.Bool),
				decls.NewInstanceOverload("overlaps_int_int",
					[]*exprpb.Type{decls.NewObjectType("pbuf.UniAttr"), decls.Int, decls.Int},
					decls.Bool)),
		),
	)

	funcs := cel.Functions(
		&functions.Overload{
			Operator: "overlaps_int_int",
			Function: func(args ...ref.Val) ref.Val {
				fmt.Println("afdasdf")
				fmt.Println(args[0].Type())
				a := args[0].Value().(*pbuf.UniAttr)
				fmt.Println(a.Start)

				fmt.Println("dfasdf", args[1].Type())
				arg1 := args[1].Value().(int64)
				fmt.Printf("tamer gur")
				fmt.Printf("tamer gur %v\n", arg1)

				return typescelgo.Bool(false)
				// return typescelgo.String(
				// 	fmt.Sprintf("Hello %s! Nice to meet you, I'm %s.\n", rhs, lhs))
			}},
	)

	//parsed, issues := env.Parse(`u.accessions.exists(a,a=="Triosephosphate isomerase")`)
	//parsed, issues := env.Parse(`overlaps(23,23)`)
	parsed, issues := env.Parse(`u.overlaps(23,23)`)
	// parsed, issues := env.Parse(`size(["tamer","gur"])>3`)
	if issues != nil && issues.Err() != nil {
		log.Fatalf("parse error: %s", issues.Err())
	}

	checked, issues := env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		log.Fatalf("type-check error: %s", issues.Err())
	}

	prg, err := env.Program(checked, funcs)
	if err != nil {
		log.Fatalf("program construction error: %s", err)
	}

	//out, details, err := prg.Eval(map[string]interface{}{"u.name": "tamer2"})
	//out, _, err := prg.Eval(&pbuf.UniAttr{Name: "tamer2", Start: 199})
	out, _, _ := prg.Eval(map[string]interface{}{"u": &pbuf.UniAttr{Accessions: []string{"Triosephosphate isomerase"},
		Start: 199}})

	fmt.Println(out) // 'true'

	// t := time.Now()
	// zone, offset := t.Zone()
	// fmt.Println(t.Location())
	// fmt.Println(zone, offset)

}
