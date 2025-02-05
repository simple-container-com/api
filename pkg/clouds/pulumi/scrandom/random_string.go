package scrandom

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
)

type String struct {
	sdk.ResourceState

	Result sdk.StringOutput `pulumi:"result"`
}

type StringArgs struct {
	Size sdk.IntInput `pulumi:"size"`
}

func NewString(ctx *sdk.Context, name string, args *StringArgs, opts ...sdk.ResourceOption) (*String, error) {
	output := &String{}

	err := ctx.RegisterComponentResource("simple-container.com:random:String", name, output, opts...)
	if err != nil {
		return nil, err
	}

	err = ctx.RegisterResourceOutputs(output, sdk.Map{
		"result": args.Size.ToIntOutput().ApplyT(func(size int) string {
			return lo.RandomString(size, lo.LettersCharset)
		}),
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}
