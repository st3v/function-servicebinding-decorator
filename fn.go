package main

import (
	"bytes"
	"context"

	providerv1alpha1 "github.com/crossplane-contrib/provider-kubernetes/apis/object/v1alpha1"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"

	v1alpha1 "github.com/st3v/servicebinding-decorator/input/v1alpha1"
)

// Function returns whatever response you ask it to.
type Function struct {
	fnv1beta1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
}

func init() {
	// register provideralpha1.Object with the runtime scheme
	providerv1alpha1.SchemeBuilder.AddToScheme(composed.Scheme)
}

// RunFunction runs the Function.
func (f *Function) RunFunction(_ context.Context, req *fnv1beta1.RunFunctionRequest) (*fnv1beta1.RunFunctionResponse, error) {
	f.log.Info("Running Servicebinding Decorator", "tag", req.GetMeta().GetTag())

	// This creates a new response to the supplied request. Note that Functions
	// are run in a pipeline! Other Functions may have run before this one. If
	// they did, response.To will copy their desired state from req to rsp. Be
	// sure to pass through any desired state your Function is not concerned
	// with unmodified.
	rsp := response.To(req, response.DefaultTTL)

	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composite resource from %T", req))
		return rsp, nil
	}
	log := f.log.WithValues(
		"xr-apiversion", oxr.Resource.GetAPIVersion(),
		"xr-kind", oxr.Resource.GetKind(),
		"xr-name", oxr.Resource.GetName(),
	)

	decorator := &v1alpha1.Decorator{}
	if err := request.GetInput(req, decorator); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get Function input from %T", req))
		return rsp, nil
	}

	claim := oxr.Resource.GetClaimReference()
	if claim == nil {
		log.Info("claim reference is nil, nothing to do", "tag", req.GetMeta().GetTag())
		response.Normal(rsp, "claim reference is nil, nothing to do")
		return rsp, nil
	}

	connSecretRef := oxr.Resource.GetWriteConnectionSecretToReference()
	if connSecretRef != nil && connSecretRef.Namespace == claim.Namespace {
		// looks like the claim specified a secret to write the connection details to
		// and that secret is in the same namespace as the claim
		// we can just refer to that secret
		return setStatusBindingName(connSecretRef.Name, req, rsp), nil
	}

	// do we require the claim to specify a secret to write the connection details to?
	if decorator.Config.RequireWriteConnectionSecretToRef {
		// note, we do not treat this as an error, the claim is simply not bindable in this case
		return f.normalAndDebug(rsp, "claim does not specify spec.writeConnectionSecretToRef, nothing to do"), nil
	}

	observed, err := request.GetObservedComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composed resources from %T", req))
		return rsp, nil
	}

	connectionDetails := map[string][]byte{}
	for _, ocr := range observed {
		for k, v := range ocr.ConnectionDetails {
			connectionDetails[k] = v
		}
	}

	// if len(oxr.ConnectionDetails) == 0 {
	// 	dxr, err := request.GetDesiredCompositeResource(req)
	// 	if err != nil {
	// 		response.Fatal(rsp, errors.Wrap(err, "cannot get desired composite resource"))
	// 		return rsp, nil
	// 	}

	// 	dxr.Resource.SetConditions(xpv1.Condition{
	// 		Type:    xpv1.TypeReady,
	// 		Status:  corev1.ConditionFalse,
	// 		Reason:  xpv1.ConditionReason("BindingSecretNotReady"),
	// 		Message: "EmptyConnectionDetails",
	// 	})

	// 	response.SetDesiredCompositeResource(rsp, dxr)
	// 	return f.normalAndDebug(rsp, "no connection details (yet)"), nil
	// }

	// the claim didn't specify a secret to write the connection details to
	// but we also don't require it to do so, rather it's up to us to create a secret now
	// we can't do this by setting spec.writeConnectionSecretToRef on the XR though as we are
	// only allowed to mutate the XR's status, not its spec
	// so instead we compose a new secret and created it using provider-kubernetes
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      string(oxr.Resource.GetUID()),
			Namespace: claim.Namespace,
		},
		Data: connectionDetails,
	}

	providerConfigName := "default"
	if decorator.Config.ProviderConfigRef != nil && decorator.Config.ProviderConfigRef.Name != "" {
		providerConfigName = decorator.Config.ProviderConfigRef.Name
	}

	enc := scheme.Codecs.EncoderForVersion(&json.Serializer{}, corev1.SchemeGroupVersion)
	buffer := &bytes.Buffer{}
	enc.Encode(&secret, buffer)

	// the object wrapper for the secret
	object := providerv1alpha1.Object{
		Spec: providerv1alpha1.ObjectSpec{
			ForProvider: providerv1alpha1.ObjectParameters{
				Manifest: runtime.RawExtension{
					Raw: buffer.Bytes(),
				},
			},
			ResourceSpec: providerv1alpha1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: providerConfigName,
				},
			},
		},
	}

	desiredComposed, err := request.GetDesiredComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired composed resources from %T", req))
		return rsp, nil
	}

	composed, err := composed.From(&object)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get composed resource from %T", object))
		return rsp, nil
	}

	// log object
	f.log.Info("desired", "composed", composed)

	desiredComposed["binding"] = &resource.DesiredComposed{Resource: composed}

	if err := response.SetDesiredComposedResources(rsp, desiredComposed); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources in %T", rsp))
		return rsp, nil
	}

	return setStatusBindingName(secret.Name, req, rsp), nil
}

// setStatusBindingName attempts to set status.binding.name on the desired composite in the response
// if this fails, the function adds a fatal result to the response
func setStatusBindingName(secretName string, req *fnv1beta1.RunFunctionRequest, rsp *fnv1beta1.RunFunctionResponse) *fnv1beta1.RunFunctionResponse {
	desiredComposite, err := request.GetDesiredCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired composite resource from %T", req))
		return rsp
	}

	if err := desiredComposite.Resource.SetString("status.binding.name", secretName); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composite resource in %T", req))
		return rsp
	}

	if err := response.SetDesiredCompositeResource(rsp, desiredComposite); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composite resource in %T", rsp))
	}

	return rsp
}

func (f *Function) normalAndDebug(rsp *fnv1beta1.RunFunctionResponse, msg string) *fnv1beta1.RunFunctionResponse {
	f.log.Info(msg)
	response.Normal(rsp, msg)
	return rsp
}
