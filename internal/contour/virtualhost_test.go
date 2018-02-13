// Copyright © 2018 Heptio
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package contour

import (
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/heptio/contour/internal/log/stdlog"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestVirtualHostCacheRecomputevhost(t *testing.T) {
	im := func(ings []*v1beta1.Ingress) map[metadata]*v1beta1.Ingress {
		m := make(map[metadata]*v1beta1.Ingress)
		for _, i := range ings {
			m[metadata{name: i.Name, namespace: i.Namespace}] = i
		}
		return m
	}
	tests := map[string]struct {
		vhost         string
		ingresses     map[metadata]*v1beta1.Ingress
		ingress_http  []envoy_api_v2_route.VirtualHost
		ingress_https []envoy_api_v2_route.VirtualHost
	}{
		"default backend": {
			vhost: "*",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Backend: backend("backend", intstr.FromInt(80)),
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "*",
				Domains: []string{"*"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/backend/80"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
		"name based vhost": {
			vhost: "httpbin.org",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host:             "httpbin.org",
						IngressRuleValue: ingressrulevalue(backend("httpbin-org", intstr.FromInt(80))),
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/"), // match all
					Action: clusteraction("default/httpbin-org/80"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
		"tls": {
			vhost: "httpbin.org",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					TLS: []v1beta1.IngressTLS{{
						Hosts:      []string{"httpbin.org"},
						SecretName: "secret",
					}},
					Rules: []v1beta1.IngressRule{{
						Host:             "httpbin.org",
						IngressRuleValue: ingressrulevalue(backend("httpbin-org", intstr.FromInt(80))),
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/"), // match all
					Action: clusteraction("default/httpbin-org/80"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/"), // match all
					Action: clusteraction("default/httpbin-org/80"),
				}},
			}},
		},
		"tls, no http": {
			vhost: "httpbin.org",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
					Annotations: map[string]string{
						"kubernetes.io/ingress.allow-http": "false",
					},
				},
				Spec: v1beta1.IngressSpec{
					TLS: []v1beta1.IngressTLS{{
						Hosts:      []string{"httpbin.org"},
						SecretName: "secret",
					}},
					Rules: []v1beta1.IngressRule{{
						Host:             "httpbin.org",
						IngressRuleValue: ingressrulevalue(backend("httpbin-org", intstr.FromInt(80))),
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{}, // kubernetes.io/ingress.allow-http: "false" prevents ingress_http
			ingress_https: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/"), // match all
					Action: clusteraction("default/httpbin-org/80"),
				}},
			}},
		},
		"tls, force https": {
			vhost: "httpbin.org",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
					Annotations: map[string]string{
						"ingress.kubernetes.io/force-ssl-redirect": "true",
					},
				},
				Spec: v1beta1.IngressSpec{
					TLS: []v1beta1.IngressTLS{{
						Hosts:      []string{"httpbin.org"},
						SecretName: "secret",
					}},
					Rules: []v1beta1.IngressRule{{
						Host:             "httpbin.org",
						IngressRuleValue: ingressrulevalue(backend("httpbin-org", intstr.FromInt(80))),
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/"), // match all
					Action: clusteraction("default/httpbin-org/80"),
				}},
				RequireTls: envoy_api_v2_route.VirtualHost_ALL,
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/"), // match all
					Action: clusteraction("default/httpbin-org/80"),
				}},
			}},
		},
		"regex vhost without match characters": {
			vhost: "httpbin.org",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "httpbin.org",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path:    "/ip", // this field _is_ a regex
									Backend: *backend("httpbin-org", intstr.FromInt(80)),
								}},
							},
						},
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/ip"), // if the field does not contact any regex characters, we treat it as a prefix
					Action: clusteraction("default/httpbin-org/80"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
		"regex vhost with match characters": {
			vhost: "httpbin.org",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "httpbin.org",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path:    "/get.*", // this field _is_ a regex
									Backend: *backend("httpbin-org", intstr.FromInt(80)),
								}},
							},
						},
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  regexmatch("/get.*"),
					Action: clusteraction("default/httpbin-org/80"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
		"named service port": {
			vhost: "httpbin.org",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host:             "httpbin.org",
						IngressRuleValue: ingressrulevalue(backend("httpbin-org", intstr.FromString("http"))),
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/httpbin-org/http"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
		"multiple routes": {
			vhost: "httpbin.org",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "httpbin.org",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path:    "/peter",
									Backend: *backend("peter", intstr.FromInt(80)),
								}, {
									Path:    "/paul",
									Backend: *backend("paul", intstr.FromString("paul")),
								}},
							},
						},
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/peter"),
					Action: clusteraction("default/peter/80"),
				}, {
					Match:  prefixmatch("/paul"),
					Action: clusteraction("default/paul/paul"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
		"multiple rules (httpbin.org)": {
			vhost: "httpbin.org",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host:             "httpbin.org",
						IngressRuleValue: ingressrulevalue(backend("peter", intstr.FromInt(80))),
					}, {
						Host:             "admin.httpbin.org",
						IngressRuleValue: ingressrulevalue(backend("paul", intstr.FromString("paul"))),
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/peter/80"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
		"multiple rules (admin.httpbin.org)": {
			vhost: "admin.httpbin.org",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host:             "httpbin.org",
						IngressRuleValue: ingressrulevalue(backend("peter", intstr.FromInt(80))),
					}, {
						Host:             "admin.httpbin.org",
						IngressRuleValue: ingressrulevalue(backend("paul", intstr.FromString("paul"))),
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "admin.httpbin.org",
				Domains: []string{"admin.httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/paul/paul"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
		"vhost name exceeds 60 chars": { // heptio/contour#25
			vhost: "my-very-very-long-service-host-name.subdomain.boring-dept.my.company",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-service-name",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host:             "my-very-very-long-service-host-name.subdomain.boring-dept.my.company",
						IngressRuleValue: ingressrulevalue(backend("my-service-name", intstr.FromInt(80))),
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "d31bb322ca62bb395acad00b3cbf45a3aa1010ca28dca7cddb4f7db786fa",
				Domains: []string{"my-very-very-long-service-host-name.subdomain.boring-dept.my.company"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/"),
					Action: clusteraction("default/my-service-name/80"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
		"second ingress object extends an existing vhost": {
			vhost: "httpbin.org",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin-admin",
					Namespace: "kube-system",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "httpbin.org",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path:    "/admin",
									Backend: *backend("admin", intstr.FromString("admin")),
								}},
							},
						},
					}},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "httpbin.org",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path:    "/",
									Backend: *backend("default", intstr.FromInt(80)),
								}},
							},
						},
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.org",
				Domains: []string{"httpbin.org"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/admin"),
					Action: clusteraction("kube-system/admin/admin"),
				}, {
					Match:  prefixmatch("/"),
					Action: clusteraction("default/default/80"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
		// kube-lego uses a single vhost in its own namespace to insert its
		// callback route for let's encrypt support.
		"kube-lego style extend vhost definitions": {
			vhost: "httpbin.davecheney.com",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-lego-nginx",
					Namespace: "kube-lego",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "httpbin.davecheney.com",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path:    "/.well-known/acme-challenge",
									Backend: *backend("kube-lego-nginx", intstr.FromInt(8080)),
								}},
							},
						},
					}, {
						Host: "httpbin2.davecheney.com",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path:    "/.well-known/acme-challenge",
									Backend: *backend("kube-lego-nginx", intstr.FromInt(8080)),
								}},
							},
						},
					}},
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httpbin",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						Host: "httpbin.davecheney.com",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path:    "/",
									Backend: *backend("httpbin", intstr.FromInt(80)),
								}},
							},
						},
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "httpbin.davecheney.com",
				Domains: []string{"httpbin.davecheney.com"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/.well-known/acme-challenge"),
					Action: clusteraction("kube-lego/kube-lego-nginx/8080"),
				}, {
					Match:  prefixmatch("/"),
					Action: clusteraction("default/httpbin/80"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
		"IngressRuleValue without host should become the default vhost": { // heptio/contour#101
			vhost: "*",
			ingresses: im([]*v1beta1.Ingress{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hello",
					Namespace: "default",
				},
				Spec: v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{{
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{{
									Path: "/hello",
									Backend: v1beta1.IngressBackend{
										ServiceName: "hello",
										ServicePort: intstr.FromInt(80),
									},
								}},
							},
						},
					}},
				},
			}}),
			ingress_http: []envoy_api_v2_route.VirtualHost{{
				Name:    "*",
				Domains: []string{"*"},
				Routes: []envoy_api_v2_route.Route{{
					Match:  prefixmatch("/hello"),
					Action: clusteraction("default/hello/80"),
				}},
			}},
			ingress_https: []envoy_api_v2_route.VirtualHost{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			const NOFLAGS = 1 << 16
			tr := &Translator{
				Logger: stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS),
			}
			tr.recomputevhost(tc.vhost, tc.ingresses)
			got := tr.VirtualHostCache.HTTP.Values()
			if !reflect.DeepEqual(tc.ingress_http, got) {
				t.Fatalf("recomputevhost(%v):\n (ingress_http) want:\n%+v\n got:\n%+v", tc.vhost, tc.ingress_http, got)
			}

			got = tr.VirtualHostCache.HTTPS.Values()
			if !reflect.DeepEqual(tc.ingress_https, got) {
				t.Fatalf("recomputevhost(%v):\n (ingress_https) want:\n%#v\ngot:\n%#v", tc.vhost, strip(tc.ingress_https), strip(got))
			}
		})
	}
}

func strip(v []envoy_api_v2_route.VirtualHost) (r []envoy_api_v2_route.VirtualHost) {
	for _, v := range v {
		r = append(r, v)
	}
	return
}

func TestValidTLSSpecforVhost(t *testing.T) {
	tests := map[string]struct {
		vhost string
		ing   v1beta1.Ingress
		want  bool
	}{
		"default vhost": {
			vhost: "*",
			ing: v1beta1.Ingress{
				Spec: v1beta1.IngressSpec{},
			},
			want: false,
		},
		"tls enabled": {
			vhost: "httpbin.davecheney.com",
			ing: v1beta1.Ingress{
				Spec: v1beta1.IngressSpec{
					TLS: []v1beta1.IngressTLS{{
						Hosts:      []string{"httpbin.davecheney.com"},
						SecretName: "httpbin",
					}},
				},
			},
			want: true,
		},
		"wrong hostname": {
			vhost: "httpbin.davecheney.com",
			ing: v1beta1.Ingress{
				Spec: v1beta1.IngressSpec{
					TLS: []v1beta1.IngressTLS{{
						Hosts:      []string{"www.davecheney.com"},
						SecretName: "dubdubdub",
					}},
				},
			},
			want: false,
		},
		"missing secret spec": {
			vhost: "httpbin.davecheney.com",
			ing: v1beta1.Ingress{
				Spec: v1beta1.IngressSpec{
					TLS: []v1beta1.IngressTLS{{
						Hosts: []string{"httpbin.davecheney.com"},
					}},
				},
			},
			want: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := validTLSSpecforVhost(tc.vhost, &tc.ing)
			if got != tc.want {
				t.Fatal("got", got, "want", tc.want)
			}
		})
	}
}

func TestGetRequestTimeout(t *testing.T) {
	tests := map[string]struct {
		a    map[string]string
		want time.Duration
		ok   bool
	}{
		"nada": {
			a:    nil,
			want: 0,
			ok:   false,
		},
		"empty": {
			a:    map[string]string{requestTimeout: ""}, // not even sure this is possible via the API
			want: 0,
			ok:   false,
		},
		"infinity": {
			a:    map[string]string{requestTimeout: "infinity"},
			want: 0,
			ok:   true,
		},
		"10 seconds": {
			a:    map[string]string{requestTimeout: "10s"},
			want: 10 * time.Second,
			ok:   true,
		},
		"invalid": {
			a:    map[string]string{requestTimeout: "10"}, // 10 what?
			want: 0,
			ok:   true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := getRequestTimeout(tc.a)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("getRequestTimeout(%q): want: %v, %v, got: %v, %v", tc.a, tc.want, tc.ok, got, ok)
			}
		})
	}
}

// clusteraction returns a route action for the supplied cluster name.
// clusteraction returns a Route_Route action for the supplied cluster.
func clusteraction(cluster string) *envoy_api_v2_route.Route_Route {
	return &envoy_api_v2_route.Route_Route{
		Route: &envoy_api_v2_route.RouteAction{
			ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}
}

// clusteractiontimeout returns a cluster action with the specified timeout.
// A timeout of 0 means infinity. If you do not want to specify a timeout, use
// clusteraction instead.
func clusteractiontimeout(name string, timeout time.Duration) *envoy_api_v2_route.Route_Route {
	// TODO(cmaloney): Pull timeout off of the backend cluster annotation
	// and use it over the value retrieved from the ingress annotation if
	// specified.
	c := clusteraction(name)
	c.Route.Timeout = &timeout
	return c
}
