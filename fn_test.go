package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"
)

func TestRunFunction(t *testing.T) {

	type args struct {
		ctx context.Context
		req *fnv1beta1.RunFunctionRequest
	}
	type want struct {
		rsp *fnv1beta1.RunFunctionResponse
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ResponseIsReturned": {
			reason: "The Function should return the expected result",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Meta: &fnv1beta1.RequestMeta{Tag: "hello"},
					Input: resource.MustStructJSON(`{
						"apiVersion": "fn.crossplane.servicebinding.io",
						"kind": "Decorator",
						"config": {
							"requireWriteConnectionSecretToRef": true,
							"providerConfigName": "my-provider-config"
						}
					}`),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							ConnectionDetails: map[string][]byte{
								"username": []byte("foo"),
								"password": []byte("bar"),
							},
							Resource: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"metadata": structpb.NewStructValue(&structpb.Struct{
										Fields: map[string]*structpb.Value{
											"uid": structpb.NewStringValue("my-uid"),
										},
									}),
									"spec": structpb.NewStructValue(&structpb.Struct{
										Fields: map[string]*structpb.Value{
											"claimRef": structpb.NewStructValue(&structpb.Struct{
												Fields: map[string]*structpb.Value{
													"name":      structpb.NewStringValue("my-claim"),
													"namespace": structpb.NewStringValue("my-claim-namespace"),
												},
											}),
										},
									}),
								},
							},
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Tag: "hello", Ttl: durationpb.New(response.DefaultTTL)},
					Results: []*fnv1beta1.Result{
						{
							Severity: fnv1beta1.Severity_SEVERITY_NORMAL,
							Message:  "I was run with input \"Hello, world!\"",
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &Function{log: logging.NewLogrLogger(NewStdoutLogger())}
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			if diff := cmp.Diff(tc.want.rsp, rsp, protocmp.Transform()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want rsp, +got rsp:\n%s", tc.reason, diff)

				fmt.Printf("\n================\nRESPONSE:\n%+v\n\n", rsp)
			}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}

// NewStdoutLogger returns a logr.Logger that prints to stdout.
func NewStdoutLogger() logr.Logger {
	return funcr.New(func(prefix, args string) {
		if prefix != "" {
			fmt.Printf("%s: %s\n", prefix, args)
		} else {
			fmt.Println(args)
		}
	}, funcr.Options{})
}
