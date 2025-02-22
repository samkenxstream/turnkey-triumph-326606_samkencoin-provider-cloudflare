package cloudflare

import (
	"context"
	"fmt"
	"log"
	"strings"

	cloudflare "github.com/cloudflare/cloudflare-go"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
)

func resourceCloudflareRateLimit() *schema.Resource {
	return &schema.Resource{
		Schema: resourceCloudflareRateLimitSchema(),
		Create: resourceCloudflareRateLimitCreate,
		Read:   resourceCloudflareRateLimitRead,
		Update: resourceCloudflareRateLimitUpdate,
		Delete: resourceCloudflareRateLimitDelete,
		Importer: &schema.ResourceImporter{
			State: resourceCloudflareRateLimitImport,
		},
	}
}

func resourceCloudflareRateLimitCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*cloudflare.API)

	zoneID := d.Get("zone_id").(string)

	rateLimitAction, err := expandRateLimitAction(d)
	if err != nil {
		return errors.Wrap(err, "error expanding rate limit action")
	}

	newRateLimit := cloudflare.RateLimit{
		Threshold: d.Get("threshold").(int),
		Period:    d.Get("period").(int),
		Action:    rateLimitAction,
	}

	newRateLimitMatch, err := expandRateLimitTrafficMatcher(d)
	if err != nil {
		return err
	}
	newRateLimit.Match = newRateLimitMatch

	if disabled, ok := d.GetOk("disabled"); ok {
		newRateLimit.Disabled = disabled.(bool)
	}

	if description, ok := d.GetOk("description"); ok {
		newRateLimit.Description = description.(string)
	}

	if bypassUrlPatterns, ok := d.GetOk("bypass_url_patterns"); ok {
		newRateLimit.Bypass = expandRateLimitBypass(bypassUrlPatterns.(*schema.Set))
	}

	newRateLimit.Correlate, _ = expandRateLimitCorrelate(d)

	newRateLimitAction, err := expandRateLimitAction(d)
	if err != nil {
		return err
	}
	newRateLimit.Action = newRateLimitAction

	log.Printf("[DEBUG] Creating Cloudflare Rate Limit from struct: %+v", newRateLimit)

	r, err := client.CreateRateLimit(context.Background(), zoneID, newRateLimit)
	if err != nil {
		return errors.Wrap(err, "error creating rate limit for zone")
	}

	if r.ID == "" {
		return fmt.Errorf("cailed to find id in Create response; resource was empty")
	}

	d.SetId(r.ID)

	log.Printf("[INFO] Cloudflare Rate Limit ID: %s", d.Id())

	return resourceCloudflareRateLimitRead(d, meta)
}

func resourceCloudflareRateLimitUpdate(d *schema.ResourceData, meta interface{}) error {
	// since api only supports replace, update looks a lot like create...
	client := meta.(*cloudflare.API)
	zoneID := d.Get("zone_id").(string)
	rateLimitId := d.Id()

	rateLimitAction, err := expandRateLimitAction(d)
	if err != nil {
		return errors.Wrap(err, "error expanding rate limit action")
	}

	updatedRateLimit := cloudflare.RateLimit{
		Threshold: d.Get("threshold").(int),
		Period:    d.Get("period").(int),
		Action:    rateLimitAction,
	}

	newRateLimitAction, err := expandRateLimitAction(d)
	if err != nil {
		return err
	}
	updatedRateLimit.Action = newRateLimitAction

	newRateLimitMatch, err := expandRateLimitTrafficMatcher(d)
	if err != nil {
		return err
	}
	updatedRateLimit.Match = newRateLimitMatch

	if disabled, ok := d.GetOk("disabled"); ok {
		updatedRateLimit.Disabled = disabled.(bool)
	}

	if description, ok := d.GetOk("description"); ok {
		updatedRateLimit.Description = description.(string)
	}

	if bypassUrlPatterns, ok := d.GetOk("bypass_url_patterns"); ok {
		updatedRateLimit.Bypass = expandRateLimitBypass(bypassUrlPatterns.(*schema.Set))
	}

	updatedRateLimit.Correlate, _ = expandRateLimitCorrelate(d)

	_, err = client.UpdateRateLimit(context.Background(), zoneID, rateLimitId, updatedRateLimit)
	if err != nil {
		return errors.Wrap(err, "error creating rate limit for zone")
	}
	return resourceCloudflareRateLimitRead(d, meta)
}

func expandRateLimitTrafficMatcher(d *schema.ResourceData) (matcher cloudflare.RateLimitTrafficMatcher, err error) {
	v, ok := d.GetOk("match")
	if !ok {
		return
	}
	cfg := v.([]interface{})[0].(map[string]interface{})

	if matchReqIface, ok := cfg["request"]; ok && len(matchReqIface.([]interface{})) > 0 {
		matchReq := matchReqIface.([]interface{})[0].(map[string]interface{})

		requestMatcher := cloudflare.RateLimitRequestMatcher{
			URLPattern: matchReq["url_pattern"].(string),
		}

		if methodsSet, ok := matchReq["methods"]; ok {
			methods := make([]string, methodsSet.(*schema.Set).Len())
			for i, m := range methodsSet.(*schema.Set).List() {
				methods[i] = m.(string)
			}
			requestMatcher.Methods = methods
		}
		if schemesSet, ok := matchReq["schemes"]; ok {
			schemes := make([]string, schemesSet.(*schema.Set).Len())
			for i, s := range schemesSet.(*schema.Set).List() {
				schemes[i] = s.(string)
			}
			requestMatcher.Schemes = schemes
		}
		matcher.Request = requestMatcher
	}
	if matchRespIface, ok := cfg["response"]; ok && len(matchRespIface.([]interface{})) > 0 {
		matchResp := matchRespIface.([]interface{})[0].(map[string]interface{})

		responseMatcher := cloudflare.RateLimitResponseMatcher{}

		if statusesSet, ok := matchResp["statuses"]; ok {
			statuses := make([]int, statusesSet.(*schema.Set).Len())
			for i, s := range statusesSet.(*schema.Set).List() {
				statuses[i] = s.(int)
			}
			responseMatcher.Statuses = statuses
		}

		if originIface, ok := matchResp["origin_traffic"]; ok {
			originTraffic := originIface.(bool)
			responseMatcher.OriginTraffic = &originTraffic
		}

		if headers, ok := matchResp["headers"]; ok {
			headersArray := make([]cloudflare.RateLimitResponseMatcherHeader, len(headers.([]interface{})))
			for i, entry := range headers.([]interface{}) {
				e := entry.(map[string]interface{})
				headersArray[i] = cloudflare.RateLimitResponseMatcherHeader{
					Name:  e["name"].(string),
					Op:    e["op"].(string),
					Value: e["value"].(string),
				}
			}
			responseMatcher.Headers = headersArray
		}

		matcher.Response = responseMatcher
	}
	return
}

func expandRateLimitAction(d *schema.ResourceData) (action cloudflare.RateLimitAction, err error) {
	// dont need to guard for array length because MinItems is set **and** action is required
	tfAction := d.Get("action").([]interface{})[0].(map[string]interface{})

	mode := tfAction["mode"].(string)
	timeout := tfAction["timeout"].(int)

	if timeout == 0 {
		if mode == "simulate" || mode == "ban" {
			return action, fmt.Errorf("rate limit timeout must be set if the 'mode' is simulate or ban")
		}
	} else if mode == "challenge" || mode == "js_challenge" {
		return action, fmt.Errorf("rate limit timeout must not be set if the 'mode' is challenge or js_challenge")
	}

	action.Mode = mode
	action.Timeout = timeout

	if _, ok := tfAction["response"]; ok && len(tfAction["response"].([]interface{})) > 0 {
		log.Printf("[DEBUG] Cloudflare Rate Limit specified action: %+v \n", tfAction)
		tfActionResponse := tfAction["response"].([]interface{})[0].(map[string]interface{})

		action.Response = &cloudflare.RateLimitActionResponse{
			ContentType: tfActionResponse["content_type"].(string),
			Body:        tfActionResponse["body"].(string),
		}
	}
	return action, nil
}

func expandRateLimitCorrelate(d *schema.ResourceData) (correlate *cloudflare.RateLimitCorrelate, err error) {
	v, ok := d.GetOk("correlate")
	if !ok {
		return
	}

	tfCorrelate := v.([]interface{})[0].(map[string]interface{})

	correlate = &cloudflare.RateLimitCorrelate{
		By: tfCorrelate["by"].(string),
	}

	return
}

func expandRateLimitBypass(bypassUrlPatterns *schema.Set) []cloudflare.RateLimitKeyValue {
	bypass := make([]cloudflare.RateLimitKeyValue, bypassUrlPatterns.Len())
	for i, urlPattern := range bypassUrlPatterns.List() {
		bypass[i] = cloudflare.RateLimitKeyValue{
			Name:  "url",
			Value: urlPattern.(string),
		}
	}
	return bypass
}

func resourceCloudflareRateLimitRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*cloudflare.API)
	zoneID := d.Get("zone_id").(string)
	rateLimitId := d.Id()

	rateLimit, err := client.RateLimit(context.Background(), zoneID, rateLimitId)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP status 404") {
			log.Printf("[INFO] Resource %s in zone %s no longer exists", rateLimitId, zoneID)
			d.SetId("")
			return nil
		} else {
			return errors.Wrap(err,
				fmt.Sprintf("Error reading rate limit resource from API for resource %s in zone %s", zoneID, rateLimitId))
		}
	}
	log.Printf("[DEBUG] Read Cloudflare Rate Limit from API as struct: %+v", rateLimit)

	d.Set("threshold", rateLimit.Threshold)
	d.Set("period", rateLimit.Period)
	if err := d.Set("match", flattenRateLimitTrafficMatcher(rateLimit.Match)); err != nil {
		log.Printf("[WARN] Error setting match on rate limit %q: %s", d.Id(), err)
	}
	if err := d.Set("action", flattenRateLimitAction(rateLimit.Action)); err != nil {
		log.Printf("[WARN] Error setting action on rate limit %q: %s", d.Id(), err)
	}

	if rateLimit.Correlate != nil {
		d.Set("correlate", flattenRateLimitCorrelate(*rateLimit.Correlate))
	}

	d.Set("description", rateLimit.Description)
	d.Set("disabled", rateLimit.Disabled)

	bypassUrlPatterns := make([]string, 0)
	for _, bypassItem := range rateLimit.Bypass {
		if bypassItem.Name == "url" {
			bypassUrlPatterns = append(bypassUrlPatterns, bypassItem.Value)
		} else {
			// maybe a new type of bypass was added to api
			log.Printf("[WARN] Unkown bypass type found in rate limit for zone %q: %s", d.Id(), bypassItem.Name)
		}
	}
	if err := d.Set("bypass_url_patterns", bypassUrlPatterns); err != nil {
		log.Printf("[WARN] Error setting bypass_url_patterns on rate limit %q: %s", d.Id(), err)
	}

	return nil
}

func flattenRateLimitTrafficMatcher(cfg cloudflare.RateLimitTrafficMatcher) []map[string]interface{} {
	data := map[string]interface{}{
		"request":  flattenRateLimitRequestMatcher(cfg.Request),
		"response": flattenRateLimitResponseMatcher(cfg.Response),
	}
	return []map[string]interface{}{data}
}

func flattenRateLimitRequestMatcher(cfg cloudflare.RateLimitRequestMatcher) []map[string]interface{} {
	data := map[string]interface{}{
		"methods":     schema.NewSet(schema.HashString, flattenStringList(cfg.Methods)),
		"schemes":     schema.NewSet(schema.HashString, flattenStringList(cfg.Schemes)),
		"url_pattern": cfg.URLPattern,
	}

	return []map[string]interface{}{data}
}

func flattenRateLimitResponseMatcher(cfg cloudflare.RateLimitResponseMatcher) []map[string]interface{} {
	data := map[string]interface{}{}

	if cfg.OriginTraffic != nil {
		data["origin_traffic"] = *cfg.OriginTraffic
	} else {
		data["origin_traffic"] = false
	}

	if len(cfg.Statuses) > 0 {
		data["statuses"] = schema.NewSet(IntIdentity, flattenIntList(cfg.Statuses))
	}

	if len(cfg.Headers) > 0 {
		headers := make([]map[string]interface{}, len(cfg.Headers))
		for i, header := range cfg.Headers {
			headers[i] = map[string]interface{}{
				"name":  header.Name,
				"op":    header.Op,
				"value": header.Value,
			}
		}
		data["headers"] = headers
	}

	if len(data) > 0 {
		return []map[string]interface{}{data}
	} else {
		return []map[string]interface{}{}
	}
}

func flattenRateLimitAction(cfg cloudflare.RateLimitAction) []map[string]interface{} {
	action := map[string]interface{}{
		"mode":    cfg.Mode,
		"timeout": cfg.Timeout,
	}

	if cfg.Response != nil {
		cfgResponse := *cfg.Response
		actionResponse := map[string]interface{}{
			"content_type": cfgResponse.ContentType,
			"body":         cfgResponse.Body,
		}
		action["response"] = []map[string]interface{}{actionResponse}
	}
	return []map[string]interface{}{action}
}

func flattenRateLimitCorrelate(cfg cloudflare.RateLimitCorrelate) []map[string]interface{} {
	correlate := map[string]interface{}{
		"by": cfg.By,
	}
	return []map[string]interface{}{correlate}
}

func resourceCloudflareRateLimitDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*cloudflare.API)
	zoneID := d.Get("zone_id").(string)
	rateLimitId := d.Id()

	log.Printf("[INFO] Deleting Cloudflare Rate Limit: %s for zone: %s", rateLimitId, zoneID)

	err := client.DeleteRateLimit(context.Background(), zoneID, rateLimitId)
	if err != nil {
		return fmt.Errorf("error deleting Cloudflare Rate Limit for zone: %s", err)
	}

	return nil
}

func resourceCloudflareRateLimitImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	// split the id so we can lookup
	idAttr := strings.SplitN(d.Id(), "/", 2)
	var zoneID string
	var rateLimitId string
	if len(idAttr) == 2 {
		zoneID = idAttr[0]
		rateLimitId = idAttr[1]
	} else {
		return nil, fmt.Errorf("invalid id (\"%s\") specified, should be in format \"zoneID/rateLimitId\" for import", d.Id())
	}

	d.Set("zone_id", zoneID)
	d.SetId(rateLimitId)

	resourceCloudflareRateLimitRead(d, meta)

	return []*schema.ResourceData{d}, nil
}
