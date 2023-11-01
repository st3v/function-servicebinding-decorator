package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/st3v/servicebinding-decorator/input/v1alpha1"
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
		"UseConnectionSecret": {
			reason: "Claim specifies spec.writeConnectionSecretToRef",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1alpha1.Decorator{
						Config: v1alpha1.Config{
							RequireWriteConnectionSecretToRef: true,
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion":"example.org/v1",
								"kind":"XR",
								"metadata":{
									"uid":"my-uid"
								},
								"spec":{
									"claimRef":{
										"name":"my-claim",
										"namespace":"my-namespace"
									},
									"writeConnectionSecretToRef":{
										"name":"my-secret",
										"namespace":"my-namespace"
									}
								}
							}`),
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion":"example.org/v1",
								"kind":"XR",
								"status":{
									"binding":{
										"name":"my-secret"
									}
								}
							}`),
						},
						Resources: map[string]*fnv1beta1.Resource{},
					},
				},
			},
		},
		"RenderNewSecret": {
			reason: "Claim does not specify spec.writeConnectionSecretToRef",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1alpha1.Decorator{
						Config: v1alpha1.Config{
							RequireWriteConnectionSecretToRef: false,
							ProviderConfigRef: &v1alpha1.ProviderConfigRef{
								Name: "my-provider-config",
							},
							BindingSecretOverrides: map[string]string{
								"type": "my-database",
							},
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion":"example.org/v1",
								"kind":"XR",
								"metadata":{
									"uid":"my-uid"
								},
								"spec":{
									"claimRef":{
										"name":"my-claim"
									}
								}
							}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"database": {
								ConnectionDetails: map[string][]byte{
									"username": []byte("their-user"),
									"password": []byte("their-password"),
									"type":     []byte("their-type"),
								},
							},
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion":"example.org/v1","kind":"XR"}`),
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion":"example.org/v1",
								"kind":"XR",
								"status":{
									"binding":{
										"name":"my-uid"
									}
								}
							}`),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"bindingsecret": {
								Resource: resource.MustStructJSON(`{
									"apiVersion":"kubernetes.crossplane.io/v1alpha1",
									"kind":"Object",
									"spec":{
										"forProvider":{
											"manifest":{
												"apiVersion":"v1",
												"kind":"Secret",
												"metadata":{
													"name": "my-uid",
													"creationTimestamp":null
												},
												"data":{
													"password":"dGhlaXItcGFzc3dvcmQ=",
													"username":"dGhlaXItdXNlcg==",
													"type":"bXktZGF0YWJhc2U="
												}
											}
										},
										"providerConfigRef":{
											"name":"my-provider-config"
										}
									}
								}`),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &Function{log: logging.NewNopLogger()}
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			if diff := cmp.Diff(tc.want.rsp, rsp, protocmp.Transform()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want rsp, +got rsp:\n%s", tc.reason, diff)
				fmt.Println(diff)
			}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}
