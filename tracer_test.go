package zipkin

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/openzipkin/zipkin-go/idgenerator"
	"github.com/openzipkin/zipkin-go/model"
	"github.com/openzipkin/zipkin-go/reporter/recorder"
)

func TestTracerOptionLocalEndpoint(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec, WithLocalEndpoint(nil))

	if want, have := ErrInvalidEndpoint, err; want != have {
		t.Errorf("expected tracer creation failure: want %+v, have: %+v", want, have)
	}

	if tr != nil {
		t.Errorf("expected tracer to be nil got: %+v", tr)
	}

	wantEP, err := NewEndpoint("testService", "localhost:80")

	if err != nil {
		t.Fatalf("expected valid endpoint, got error: %+v", err)
	}

	tr, err = NewTracer(rec, WithLocalEndpoint(wantEP))

	if err != nil {
		t.Fatalf("expected valid tracer, got error: %+v", err)
	}

	if tr == nil {
		t.Error("expected valid tracer, got nil")
	}

	haveEP := tr.LocalEndpoint()

	if want, have := wantEP.ServiceName, haveEP.ServiceName; want != have {
		t.Errorf("ServiceName want %s, have %s", want, have)
	}

	if !wantEP.IPv4.Equal(haveEP.IPv4) {
		t.Errorf(" IPv4 want %+v, have %+v", wantEP.IPv4, haveEP.IPv4)
	}

	if !wantEP.IPv6.Equal(haveEP.IPv6) {
		t.Errorf("IPv6 want %+v, have %+v", wantEP.IPv6, haveEP.IPv6)
	}
}

func TestTracerOptionExtractFailurePolicy(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	policies := []struct {
		policy ExtractFailurePolicy
		err    error
	}{
		{-1, ErrInvalidExtractFailurePolicy},
		{ExtractFailurePolicyRestart, nil},
		{ExtractFailurePolicyError, nil},
		{ExtractFailurePolicyTagAndRestart, nil},
		{3, ErrInvalidExtractFailurePolicy},
	}

	for idx, item := range policies {
		tr, err := NewTracer(rec, WithExtractFailurePolicy(item.policy))

		if want, have := item.err, err; want != have {
			t.Errorf("[%d] expected tracer creation failure: want %+v, have %+v", idx, item.err, err)
		}

		if err != nil && tr != nil {
			t.Errorf("[%d] expected tracer to be nil, have: %+v", idx, tr)
		}
	}
}

func TestTracerIDGeneratorOption(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	gen := idgenerator.NewRandomTimestamped()

	tr, err := NewTracer(rec, WithIDGenerator(gen))

	if err != nil {
		t.Fatalf("expected valid tracer, got error: %+v", err)
	}

	if want, have := gen, tr.generate; want != have {
		t.Errorf("id generator want %+v, have %+v", want, have)
	}
}

func TestTracerWithTraceID128BitOption(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec, WithTraceID128Bit(false))

	if err != nil {
		t.Fatalf("expected valid tracer, got error: %+v", err)
	}

	if want, have := reflect.TypeOf(idgenerator.NewRandom64()), reflect.TypeOf(tr.generate); want != have {
		t.Errorf("id generator want %+v, have %+v", want, have)
	}

	tr, err = NewTracer(rec, WithTraceID128Bit(true))

	if err != nil {
		t.Fatalf("expected valid tracer, got error: %+v", err)
	}

	if want, have := reflect.TypeOf(idgenerator.NewRandom128()), reflect.TypeOf(tr.generate); want != have {
		t.Errorf("id generator want %+v, have %+v", want, have)
	}
}

func TestTracerExtractor(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec)
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	testErr1 := errors.New("extractor error")
	extractorErr := func() (*model.SpanContext, error) {
		return nil, testErr1
	}

	sc := tr.Extract(extractorErr)

	if want, have := testErr1, sc.Err; want != have {
		t.Errorf("Err want %+v, have %+v", want, have)
	}

	spanContext := model.SpanContext{}
	extractor := func() (*model.SpanContext, error) {
		return &spanContext, nil
	}

	sc = tr.Extract(extractor)

	if want, have := spanContext, sc; want != have {
		t.Errorf("SpanContext want %+v, have %+v", want, have)
	}

	if want, have := &spanContext, &sc; want == have {
		t.Error("expected different span context objects")
	}
}

func TestNoopTracer(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec)
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	pSC := model.SpanContext{
		TraceID: model.TraceID{
			High: 0,
			Low:  1,
		},
		ID: model.ID(1),
	}

	span := tr.StartSpan("test", Parent(pSC))

	if want, have := reflect.TypeOf(&spanImpl{}), reflect.TypeOf(span); want != have {
		t.Errorf("span implementation type want %+v, have %+v", want, have)
	}

	span.Finish()

	tr.SetNoop(true)

	testErr1 := errors.New("extractor error")
	extractor := func() (*model.SpanContext, error) {
		return nil, testErr1
	}

	sc := tr.Extract(extractor)

	if sc.Err != nil {
		t.Errorf("Err want nil, have %+v", sc.Err)
	}

	span = tr.StartSpan("test", Parent(pSC))

	if want, have := reflect.TypeOf(&noopSpan{}), reflect.TypeOf(span); want != have {
		t.Errorf("span implementation type want %+v, have %+v", want, have)
	}

	span.Finish()

	tr.SetNoop(false)

	span = tr.StartSpan("test", Parent(pSC))

	if want, have := reflect.TypeOf(&spanImpl{}), reflect.TypeOf(span); want != have {
		t.Errorf("span implementation type want %+v, have %+v", want, have)
	}

	span.Finish()

	tr, err = NewTracer(rec, WithNoopTracer(true))
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	testErr1 = errors.New("extractor error")
	extractor = func() (*model.SpanContext, error) {
		return nil, testErr1
	}

	sc = tr.Extract(extractor)

	if sc.Err != nil {
		t.Errorf("Err want nil, have %+v", sc.Err)
	}

	span = tr.StartSpan("test", Parent(pSC))

	if want, have := reflect.TypeOf(&noopSpan{}), reflect.TypeOf(span); want != have {
		t.Errorf("span implementation type want %+v, have %+v", want, have)
	}

	tr, err = NewTracer(rec, WithNoopTracer(false))
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	span = tr.StartSpan("test", Parent(pSC))

	if want, have := reflect.TypeOf(&spanImpl{}), reflect.TypeOf(span); want != have {
		t.Errorf("span implementation type want %+v, have %+v", want, have)
	}
}

func TestNoopSpan(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec, WithNoopSpan(true))
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	sampled := false
	pSC := model.SpanContext{
		TraceID: model.TraceID{
			High: 0,
			Low:  1,
		},
		ID:      model.ID(1),
		Sampled: &sampled,
	}

	span := tr.StartSpan("test", Parent(pSC))

	if want, have := reflect.TypeOf(&noopSpan{}), reflect.TypeOf(span); want != have {
		t.Errorf("span implementation type want %+v, have %+v", want, have)
	}

	span.Finish()
}

func TestUnsampledSpan(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec, WithTraceID128Bit(false))
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	sampled := false
	pSC := model.SpanContext{
		TraceID: model.TraceID{
			High: 0,
			Low:  1,
		},
		ID:      model.ID(1),
		Sampled: &sampled,
	}

	span := tr.StartSpan("test", Parent(pSC))

	if want, have := reflect.TypeOf(&spanImpl{}), reflect.TypeOf(span); want != have {
		t.Errorf("span implementation type want %+v, have %+v", want, have)
	}

	cSC := span.Context()

	if cSC.Err != nil {
		t.Errorf("Err want nil, have %+v", cSC.Err)
	}

	if want, have := pSC.Debug, cSC.Debug; want != have {
		t.Errorf("Debug want %t, have %t", want, have)
	}

	if want, have := pSC.TraceID, cSC.TraceID; want != have {
		t.Errorf("TraceID want %+v, have %+v", want, have)
	}

	if cSC.ID == 0 {
		t.Error("ID want valid value, have 0")
	}

	if cSC.ParentID == nil {
		t.Errorf("ParentID want %+v, have nil", pSC.ID)
	} else if want, have := pSC.ID, *cSC.ParentID; want != have {
		t.Errorf("ParentID want %+v, have %+v", want, have)
	}

	if cSC.Sampled == nil {
		t.Error("Sampled want false, have nil")
	} else if *cSC.Sampled {
		t.Errorf("Sampled want false, have %+v", *cSC.Sampled)
	}

	if want, have := int32(0), span.(*spanImpl).mustCollect; want != have {
		t.Errorf("expected mustCollect %d, got %d", want, have)
	}

	span.Finish()
}

func TestDefaultTags(t *testing.T) {
	var (
		scTagKey   = "spanScopedTag"
		scTagValue = "spanPayload"
		tags       = make(map[string]string)
	)
	tags["platform"] = "zipkin_test"
	tags["version"] = "1.0"

	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec, WithTags(tags), WithTraceID128Bit(true))
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	pSC := model.SpanContext{
		TraceID: model.TraceID{
			High: 0,
			Low:  1,
		},
		ID: model.ID(1),
	}

	span := tr.StartSpan("test", Kind(model.Server), Parent(pSC))
	span.Tag(scTagKey, scTagValue)

	foundTags := span.(*spanImpl).Tags

	for key, value := range tags {
		foundValue, foundKey := foundTags[key]
		if !foundKey {
			t.Errorf("Tag want %s=%s, have key not found", key, value)
		} else if value != foundValue {
			t.Errorf("Tag want %s=%s, have %s=%s", key, value, key, foundValue)
		}
	}

	foundValue, foundKey := foundTags[scTagKey]
	if !foundKey {
		t.Errorf("Tag want %s=%s, have key not found", scTagKey, scTagValue)
	} else if want, have := scTagValue, foundValue; want != have {
		t.Errorf("Tag want %s=%s, have %s=%s", scTagKey, scTagValue, scTagKey, foundValue)
	}
}

func TestTagOverwriteRules(t *testing.T) {
	var (
		k1      = "key1"
		v1First = "value to overwrite"
		v1Last  = "value to keep"
		k2      = string(TagError)
	)

	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec, WithIDGenerator(idgenerator.NewRandomTimestamped()))
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	s := tr.StartSpan("test_tags")
	defer s.Finish()

	s.Tag(k1, v1First)

	if want, have := v1First, s.(*spanImpl).Tags[k1]; want != have {
		t.Errorf("Tag want %s=%s, have %s=%s", k1, want, k1, have)
	}

	s.Tag(k1, v1Last)

	if want, have := v1Last, s.(*spanImpl).Tags[k1]; want != have {
		t.Errorf("Tag want %s=%s, have %s=%s", k1, want, k1, have)
	}

	s.Tag(k2, v1First)

	if want, have := v1First, s.(*spanImpl).Tags[k2]; want != have {
		t.Errorf("Tag want %s=%s, have %s=%s", k1, want, k1, have)
	}

	s.Tag(k2, v1Last)

	if want, have := v1First, s.(*spanImpl).Tags[k2]; want != have {
		t.Errorf("Tag want %s=%s, have %s=%s", k1, want, k1, have)
	}

	TagError.Set(s, v1Last)

	if want, have := v1First, s.(*spanImpl).Tags[k2]; want != have {
		t.Errorf("Tag want %s=%s, have %s=%s", k1, want, k1, have)
	}
}

func TestAnnotations(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec)
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	s := tr.StartSpan("test_tags")
	defer s.Finish()

	annotations := []model.Annotation{
		{
			Timestamp: time.Now().Add(10 * time.Millisecond),
			Value:     "annotation 1",
		},
		{
			Timestamp: time.Now().Add(20 * time.Millisecond),
			Value:     "annotation 2",
		},
		{
			Timestamp: time.Now().Add(30 * time.Millisecond),
			Value:     "annotation 3",
		},
	}

	for _, annotation := range annotations {
		s.Annotate(annotation.Timestamp, annotation.Value)
	}

	time.Sleep(40 * time.Millisecond)

	if want, have := len(annotations), len(s.(*spanImpl).Annotations); want != have {
		t.Fatalf("Annotation count want %d, have %d", want, have)
	}

	for idx, annotation := range annotations {
		if want, have := annotation, s.(*spanImpl).Annotations[idx]; want != have {
			t.Errorf("Annotation #%d want %+v, have %+v", idx, want, have)
		}
	}
}

func TestExplicitStartTime(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec, WithSampler(NewModuloSampler(2)))
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	st := time.Now()

	time.Sleep(10 * time.Millisecond)

	s := tr.StartSpan("test_tags", StartTime(st))
	defer s.Finish()

	if want, have := st, s.(*spanImpl).Timestamp; want != have {
		t.Errorf("Timestamp want %+v, have %+v", want, have)
	}
}

func TestDebugFlagWithoutParentTrace(t *testing.T) {
	/*
	   Test handling of a single Debug flag without an existing trace
	*/
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec, WithSharedSpans(true))
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	pSC := model.SpanContext{
		Debug: true,
	}

	span := tr.StartSpan("test", Parent(pSC))

	cSC := span.Context()

	if cSC.Err != nil {
		t.Errorf("Err want nil, have %+v", cSC.Err)
	}

	if want, have := pSC.Debug, cSC.Debug; want != have {
		t.Errorf("Debug want %t, have %t", want, have)
	}

	if want, have := false, cSC.TraceID.Empty(); want != have {
		t.Error("expected valid TraceID")
	}

	if cSC.ID == 0 {
		t.Error("expected valid ID")
	}

	if cSC.ParentID != nil {
		t.Errorf("ParentID want nil, have %+v", cSC.ParentID)
	}

	if cSC.Sampled != nil {
		t.Errorf("Sampled want nil, have %+v", cSC.Sampled)
	}

	if want, have := int32(1), span.(*spanImpl).mustCollect; want != have {
		t.Errorf("mustCollect want %d, have %d", want, have)
	}
}

func TestParentSpanInSharedMode(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec, WithSharedSpans(true))
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	parentID := model.ID(1)

	pSC := model.SpanContext{
		TraceID: model.TraceID{
			High: 0,
			Low:  1,
		},
		ID:       model.ID(2),
		ParentID: &parentID,
	}

	span := tr.StartSpan("test", Kind(model.Server), Parent(pSC))

	cSC := span.Context()

	if cSC.Err != nil {
		t.Errorf("Err want nil, have %+v", cSC.Err)
	}

	if want, have := pSC.Debug, cSC.Debug; want != have {
		t.Errorf("Debug want %t, have %t", want, have)
	}

	if want, have := pSC.TraceID, cSC.TraceID; want != have {
		t.Errorf("TraceID want %+v, have %+v", want, have)
	}

	if want, have := pSC.ID, cSC.ID; want != have {
		t.Errorf("ID want %+v, have %+v", want, have)
	}

	if cSC.ParentID == nil {
		t.Error("ParentID want valid value, have nil")
	} else if want, have := parentID, *cSC.ParentID; want != have {
		t.Errorf("ParentID want %+v, have %+v", want, have)
	}

	if cSC.Sampled == nil {
		t.Error("Sampled want explicit value, have nil")
	} else if !*cSC.Sampled {
		t.Errorf("Sampled want true, have %+v", *cSC.Sampled)
	}

	if want, have := int32(1), span.(*spanImpl).mustCollect; want != have {
		t.Errorf("mustCollect want %d, have %d", want, have)
	}
}

func TestParentSpanInSpanPerNodeMode(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec, WithSharedSpans(false))
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	pSC := model.SpanContext{
		TraceID: model.TraceID{
			High: 0,
			Low:  1,
		},
		ID: model.ID(1),
	}

	span := tr.StartSpan("test", Kind(model.Server), Parent(pSC))

	cSC := span.Context()

	if cSC.Err != nil {
		t.Errorf("Err want nil, have %+v", cSC.Err)
	}

	if want, have := pSC.Debug, cSC.Debug; want != have {
		t.Errorf("Debug want %t, have %t", want, have)
	}

	if want, have := pSC.TraceID, cSC.TraceID; want != have {
		t.Errorf("TraceID want %+v, have: %+v", want, have)
	}

	if cSC.ID == 0 {
		t.Error("expected valid ID")
	}

	if cSC.ParentID == nil {
		t.Error("ParentID want valid value, have nil")
	} else if want, have := pSC.ID, *cSC.ParentID; want != have {
		t.Errorf("ParentID want %+v, have %+v", want, have)
	}

	if cSC.Sampled == nil {
		t.Error("Sampled want explicit value, have nil")
	} else if !*cSC.Sampled {
		t.Errorf("Sampled want true, have %+v", *cSC.Sampled)
	}

	if want, have := int32(1), span.(*spanImpl).mustCollect; want != have {
		t.Errorf("mustCollect want %d, have %d", want, have)
	}
}

func TestStartSpanFromContext(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tr, err := NewTracer(rec, WithSharedSpans(true))
	if err != nil {
		t.Fatalf("unable to create tracer instance: %+v", err)
	}

	cSpan := tr.StartSpan("test", Kind(model.Client))

	ctx := NewContext(context.Background(), cSpan)

	sSpan, _ := tr.StartSpanFromContext(ctx, "testChild", Kind(model.Server))

	cS, sS := cSpan.(*spanImpl), sSpan.(*spanImpl)

	if want, have := model.Client, cS.Kind; want != have {
		t.Errorf("Kind want %+v, have %+v", want, have)
	}

	if want, have := model.Server, sS.Kind; want != have {
		t.Errorf("Kind want %+v, have: %+v", want, have)
	}

	if want, have := cS.TraceID, sS.TraceID; want != have {
		t.Errorf("TraceID want %+v, have: %+v", want, have)
	}

	if want, have := cS.ID, sS.ID; want != have {
		t.Errorf("ID want %+v, have %+v", want, have)
	}

	if want, have := cS.ParentID, sS.ParentID; want != have {
		t.Errorf("ParentID want %+v, have %+v", want, have)
	}
}

func TestLocalEndpoint(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tracer, err := NewTracer(rec)
	if err != nil {
		t.Fatalf("expected valid tracer, got error: %+v", err)
	}

	ep, err := NewEndpoint("my service", "localhost:80")

	if err != nil {
		t.Fatalf("expected valid endpoint, got error: %+v", err)
	}

	tracer, err = NewTracer(rec, WithLocalEndpoint(ep))
	if err != nil {
		t.Fatalf("expected valid tracer, got error: %+v", err)
	}

	want, have := ep, tracer.LocalEndpoint()

	if have == nil {
		t.Fatalf("endpoint want %+v, have nil", want)
	}

	if want.ServiceName != have.ServiceName {
		t.Errorf("serviceName want %s, have %s", want.ServiceName, have.ServiceName)
	}

	if !want.IPv4.Equal(have.IPv4) {
		t.Errorf("IPv4 endpoint want %+v, have %+v", want.IPv4, have.IPv4)
	}

	if !want.IPv6.Equal(have.IPv6) {
		t.Errorf("IPv6 endpoint want %+v, have %+v", want.IPv6, have.IPv6)
	}
}

func TestRemoteEndpoint(t *testing.T) {
	rec := recorder.NewReporter()
	defer rec.Close()

	tracer, err := NewTracer(rec)
	if err != nil {
		t.Fatalf("expected valid tracer, got error: %+v", err)
	}

	ep1, err := NewEndpoint("myService", "www.google.com:80")

	if err != nil {
		t.Fatalf("expected valid endpoint, got error: %+v", err)
	}

	span := tracer.StartSpan("test", RemoteEndpoint(ep1))

	if !reflect.DeepEqual(span.(*spanImpl).RemoteEndpoint, ep1) {
		t.Errorf("RemoteEndpoint want %+v, have %+v", ep1, span.(*spanImpl).RemoteEndpoint)
	}

	ep2, err := NewEndpoint("otherService", "www.microsoft.com:443")

	if err != nil {
		t.Fatalf("expected valid endpoint, got error: %+v", err)
	}

	span.SetRemoteEndpoint(ep2)

	if !reflect.DeepEqual(span.(*spanImpl).RemoteEndpoint, ep2) {
		t.Errorf("RemoteEndpoint want %+v, have %+v", ep1, span.(*spanImpl).RemoteEndpoint)
	}
}