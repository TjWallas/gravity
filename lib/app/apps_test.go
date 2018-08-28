package app

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/gravitational/gravity/lib/app/resources"
	"github.com/gravitational/gravity/lib/compare"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/pack"
	"github.com/gravitational/gravity/lib/systeminfo"
	"k8s.io/api/core/v1"

	"github.com/gravitational/trace"
	. "gopkg.in/check.v1"
)

func TestAppUtils(t *testing.T) { TestingT(t) }

type AppUtilsSuite struct{}

var _ = Suite(&AppUtilsSuite{})

func (s *AppUtilsSuite) TestUpdatedDependencies(c *C) {
	app1 := Application{
		Package: loc.MustParseLocator("repo/app:1.0.0"),
		PackageEnvelope: pack.PackageEnvelope{
			Manifest: []byte(app1Manifest),
		},
	}
	app2 := Application{
		Package: loc.MustParseLocator("repo/app:2.0.0"),
		PackageEnvelope: pack.PackageEnvelope{
			Manifest: []byte(app2Manifest),
		},
	}

	updates, err := GetUpdatedDependencies(app1, app2)
	c.Assert(err, IsNil)
	c.Assert(updates, DeepEquals, []loc.Locator{
		loc.MustParseLocator("repo/dep-2:2.0.0"),
		loc.MustParseLocator("repo/app:2.0.0"),
	})

	updates, err = GetUpdatedDependencies(app1, app1)
	c.Assert(trace.IsNotFound(err), Equals, true)
	c.Assert(updates, DeepEquals, []loc.Locator(nil))
}

func (s *AppUtilsSuite) TestUpdatesSecurityContext(c *C) {
	// setup
	type resource struct {
		fileName string
		data     []byte
	}
	var serviceUser = systeminfo.User{
		Name: "planet",
		UID:  1001,
		GID:  1001,
	}
	var testCases = []struct {
		input   resource
		verify  func(*C, []byte)
		comment string
	}{
		{
			input: resource{fileName: "resources.yaml", data: []byte(twoPods)},
			verify: func(c *C, data []byte) {
				res, err := resources.Decode(bytes.NewReader(data))
				c.Assert(err, IsNil)
				for _, object := range res.Objects {
					switch resource := object.(type) {
					case *v1.Pod:
						switch resource.Name {
						case "nginx":
							// This pod contains service user placeholder
							// in both pod's security context and local container's
							// security context
							verifyPodSecurityContext(c, resource.Spec.SecurityContext, serviceUser)
							for _, container := range resource.Spec.Containers {
								verifySecurityContext(c, container.SecurityContext, serviceUser)
							}
						case "foo":
							// This pod does not use service user context
							verifyPodSecurityContext(c, resource.Spec.SecurityContext, systeminfo.User{UID: 0})
						}
					default:
						c.Errorf("unexpected object of type %T", object)
					}
				}
			},
			comment: "Updates service user ID",
		},
		{
			input: resource{fileName: "resource.yaml", data: []byte(`
# this is a comment
foo:
  bar: 10`)},
			verify: func(c *C, data []byte) {
				compare.DeepCompare(c, data, []byte(`
# this is a comment
foo:
  bar: 10`))
			},
			comment: "Does not rewrite resources that weren't updated",
		},
		{
			input: resource{fileName: "resource.txt", data: []byte("unrelated resource file")},
			verify: func(c *C, data []byte) {
				compare.DeepCompare(c, data, []byte("unrelated resource file"))
			},
			comment: "Ignores unrelated files",
		},
		{
			input: resource{fileName: "resource.yaml", data: []byte("invalid YAML-formatted resource file")},
			verify: func(c *C, data []byte) {
				compare.DeepCompare(c, data, []byte("invalid YAML-formatted resource file"))
			},
			comment: "Ignores files that fail to parse",
		},
	}

	// exercise & verify
	for _, testCase := range testCases {
		dir := c.MkDir()

		err := ioutil.WriteFile(filepath.Join(dir, testCase.input.fileName), testCase.input.data, defaults.SharedReadWriteMask)
		c.Assert(err, IsNil, Commentf(testCase.comment))

		err = UpdateSecurityContextInDir(dir, serviceUser)
		c.Assert(err, IsNil, Commentf(testCase.comment))

		data, err := ioutil.ReadFile(filepath.Join(dir, testCase.input.fileName))
		c.Assert(err, IsNil, Commentf(testCase.comment))
		testCase.verify(c, data)
	}
}

func verifySecurityContext(c *C, ctx *v1.SecurityContext, user systeminfo.User) {
	uid := int64(user.UID)
	compare.DeepCompare(c, ctx, &v1.SecurityContext{RunAsUser: &uid})
}

func verifyPodSecurityContext(c *C, ctx *v1.PodSecurityContext, user systeminfo.User) {
	uid := int64(user.UID)
	compare.DeepCompare(c, ctx, &v1.PodSecurityContext{RunAsUser: &uid})
}

const twoPods = `
apiVersion: v1
kind: Pod
metadata:
  name: nginx
  labels:
    app: nginx
spec:
  securityContext:
    runAsUser: -1
  containers:
  - name: nginx
    image: nginx
    ports:
    - containerPort: 80
    securityContext:
      runAsUser: -1
---
# this resource does not use service user
apiVersion: v1
kind: Pod
metadata:
  name: foo
  labels:
    app: foo
spec:
  securityContext:
    runAsUser: 0
  containers:
  - name: foo
    image: foo:latest`

const app1Manifest = `apiVersion: bundle.gravitational.io/v2
kind: Bundle
metadata:
  name: app
  resourceVersion: 1.0.0
dependencies:
  apps:
    - repo/dep-1:1.0.0
    - repo/dep-2:1.0.0`

const app2Manifest = `apiVersion: bundle.gravitational.io/v2
kind: Bundle
metadata:
  name: app
  resourceVersion: 2.0.0
dependencies:
  apps:
    - repo/dep-1:1.0.0
    - repo/dep-2:2.0.0`
