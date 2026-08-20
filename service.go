package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131 Safari/537.36"

type SearchResult struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Channel   string `json:"channel"`
	ChannelID string `json:"channel_id"`
	Duration  string `json:"duration"`
	Thumbnail string `json:"thumbnail"`
	Published string `json:"published"`
	IsShort   bool   `json:"is_short"`
}

type SearchPage struct {
	Results []SearchResult `json:"results"`
	Cursor  *string        `json:"cursor"`
}

type Subscription struct {
	Channel   string `json:"channel"`
	ChannelID string `json:"channel_id"`
	Avatar    string `json:"avatar"`
}

type BlockedItem struct {
	Kind      string `json:"kind"`
	Value     string `json:"value"`
	Label     string `json:"label"`
	Thumbnail string `json:"thumbnail"`
}

type searchCursor struct {
	Token         string
	APIKey        string
	ClientVersion string
	VisitorData   string
}

type searchState struct {
	mu      sync.Mutex
	cursors map[string]searchCursor
}

func newSearchState() *searchState { return &searchState{cursors: make(map[string]searchCursor)} }

type YtLiteService struct {
	state   *searchState
	storage sync.Mutex
}

var nextCursor uint64

func NewYtLiteService() *YtLiteService { return &YtLiteService{state: newSearchState()} }

func (s *YtLiteService) SetInputLock(locked bool) { setInputLocked(locked) }

func dataDirectory() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(base, "land.moreno.ytlite")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	for _, legacyDirectory := range []string{filepath.Join(filepath.Dir(directory), "land.moreno.tauritube"), filepath.Join(filepath.Dir(directory), "land.moreno.youtube-tauri")} {
		for _, filename := range []string{"subscriptions.json", "blocks.json"} {
			current := filepath.Join(directory, filename)
			legacy := filepath.Join(legacyDirectory, filename)
			if _, currentErr := os.Stat(current); errors.Is(currentErr, os.ErrNotExist) {
				if _, legacyErr := os.Stat(legacy); legacyErr == nil {
					if err := copyFile(legacy, current); err != nil {
						return "", err
					}
				}
			}
		}
	}
	return directory, nil
}

func subscriptionsPath() (string, error) {
	directory, err := dataDirectory()
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, "subscriptions.json")
	return path, nil
}

func blocksPath() (string, error) {
	directory, err := dataDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "blocks.json"), nil
}

func copyFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0o644)
}

func readJSON[T any](path string, target *T) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readSubscriptions() ([]Subscription, error) {
	path, err := subscriptionsPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return []Subscription{}, nil
	}
	var subscriptions []Subscription
	if err := readJSON(path, &subscriptions); err != nil {
		return nil, err
	}
	return subscriptions, nil
}

func readBlocks() ([]BlockedItem, error) {
	path, err := blocksPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return []BlockedItem{}, nil
	}
	var blocks []BlockedItem
	if err := readJSON(path, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

func blockKey(item BlockedItem) string {
	value := strings.ToLower(item.Value)
	if item.Kind == "channel" && item.Label != "" {
		value = strings.ToLower(item.Label)
	}
	return item.Kind + ":" + value
}

func dedupeBlocks(blocks []BlockedItem) []BlockedItem {
	seen := make(map[string]struct{}, len(blocks))
	result := make([]BlockedItem, 0, len(blocks))
	for _, item := range blocks {
		if _, exists := seen[blockKey(item)]; exists {
			continue
		}
		seen[blockKey(item)] = struct{}{}
		result = append(result, item)
	}
	return result
}

func addBlock(blocks []BlockedItem, kind, value, label, thumbnail string) []BlockedItem {
	if value == "" {
		return blocks
	}
	for index := range blocks {
		item := &blocks[index]
		if item.Kind == kind && (strings.EqualFold(item.Value, value) || (kind == "channel" && label != "" && strings.EqualFold(item.Label, label))) {
			if item.Label == "" {
				item.Label = label
			}
			if item.Thumbnail == "" {
				item.Thumbnail = thumbnail
			}
			return blocks
		}
	}
	return append(blocks, BlockedItem{Kind: kind, Value: value, Label: label, Thumbnail: thumbnail})
}

func (s *YtLiteService) ListSubscriptions() ([]Subscription, error) {
	s.storage.Lock()
	defer s.storage.Unlock()
	return readSubscriptions()
}

func (s *YtLiteService) ListBlocks() ([]BlockedItem, error) {
	s.storage.Lock()
	defer s.storage.Unlock()
	blocks, err := readBlocks()
	if err != nil {
		return nil, err
	}
	blocks = dedupeBlocks(blocks)
	path, err := blocksPath()
	if err != nil {
		return nil, err
	}
	if err := writeJSON(path, blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

func (s *YtLiteService) BlockVideo(id, label, thumbnail string) error {
	if label == "" {
		label = id
	}
	if thumbnail == "" {
		thumbnail = fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", id)
	}
	s.storage.Lock()
	defer s.storage.Unlock()
	blocks, err := readBlocks()
	if err != nil {
		return err
	}
	blocks = addBlock(dedupeBlocks(blocks), "video", id, label, thumbnail)
	path, err := blocksPath()
	if err != nil {
		return err
	}
	return writeJSON(path, blocks)
}

func (s *YtLiteService) BlockChannel(channel, channelID, thumbnail string) error {
	value := channel
	if channelID != "" {
		value = channelID
	}
	s.storage.Lock()
	defer s.storage.Unlock()
	blocks, err := readBlocks()
	if err != nil {
		return err
	}
	blocks = addBlock(dedupeBlocks(blocks), "channel", value, channel, thumbnail)
	path, err := blocksPath()
	if err != nil {
		return err
	}
	return writeJSON(path, blocks)
}

func (s *YtLiteService) UnblockItem(item BlockedItem) error {
	s.storage.Lock()
	defer s.storage.Unlock()
	blocks, err := readBlocks()
	if err != nil {
		return err
	}
	filtered := blocks[:0]
	for _, block := range blocks {
		if block.Kind != item.Kind || block.Value != item.Value {
			filtered = append(filtered, block)
		}
	}
	path, err := blocksPath()
	if err != nil {
		return err
	}
	return writeJSON(path, filtered)
}

func (s *YtLiteService) SubscribeChannel(channel, channelID, avatar string) error {
	s.storage.Lock()
	defer s.storage.Unlock()
	subscriptions, err := readSubscriptions()
	if err != nil {
		return err
	}
	found := false
	for index := range subscriptions {
		item := &subscriptions[index]
		if strings.EqualFold(item.Channel, channel) {
			found = true
			if channelID != "" {
				item.ChannelID = channelID
			}
			if item.Avatar == "" {
				item.Avatar = avatar
			}
			break
		}
	}
	if !found && channel != "" {
		subscriptions = append(subscriptions, Subscription{Channel: channel, ChannelID: channelID, Avatar: avatar})
	}
	path, err := subscriptionsPath()
	if err != nil {
		return err
	}
	return writeJSON(path, subscriptions)
}

func (s *YtLiteService) UnsubscribeChannel(channel string) error {
	s.storage.Lock()
	defer s.storage.Unlock()
	subscriptions, err := readSubscriptions()
	if err != nil {
		return err
	}
	filtered := subscriptions[:0]
	for _, item := range subscriptions {
		if !strings.EqualFold(item.Channel, channel) {
			filtered = append(filtered, item)
		}
	}
	path, err := subscriptionsPath()
	if err != nil {
		return err
	}
	return writeJSON(path, filtered)
}

func text(value any) string {
	switch item := value.(type) {
	case string:
		return item
	case map[string]any:
		for _, key := range []string{"content", "simpleText"} {
			if result, ok := item[key].(string); ok {
				return result
			}
		}
		if runs, ok := item["runs"].([]any); ok && len(runs) > 0 {
			if first, ok := runs[0].(map[string]any); ok {
				if result, ok := first["text"].(string); ok {
					return result
				}
			}
		}
	}
	return ""
}

func publishedLabel(value string) bool { return strings.Contains(strings.ToLower(value), "ago") }

func isChannelMissing(value string) bool {
	lower := strings.ToLower(value)
	return value == "" || strings.Contains(lower, "views") || strings.Contains(lower, "watching") || publishedLabel(value)
}

func fillChannel(results []SearchResult, channel, channelID string) {
	for index := range results {
		if isChannelMissing(results[index].Channel) {
			results[index].Channel = channel
		}
		if results[index].ChannelID == "" {
			results[index].ChannelID = channelID
		}
	}
}

func metadataRows(value any) []any {
	switch item := value.(type) {
	case map[string]any:
		if rows, ok := item["metadataRows"].([]any); ok {
			return rows
		}
		for _, child := range item {
			if rows := metadataRows(child); rows != nil {
				return rows
			}
		}
	case []any:
		for _, child := range item {
			if rows := metadataRows(child); rows != nil {
				return rows
			}
		}
	}
	return nil
}

func lockupMetadata(lockup map[string]any) (string, string) {
	rows := metadataRows(lockup)
	if rows == nil {
		return "", ""
	}
	texts := make([]string, 0)
	for _, rowValue := range rows {
		row, ok := rowValue.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := row["metadataParts"].([]any)
		if !ok {
			continue
		}
		for _, partValue := range parts {
			part, ok := partValue.(map[string]any)
			if !ok {
				continue
			}
			if value := text(part["text"]); value != "" {
				texts = append(texts, value)
			}
		}
	}
	published := ""
	for _, value := range texts {
		if publishedLabel(value) {
			published = value
			break
		}
	}
	channel := ""
	if len(rows) > 0 {
		if row, ok := rows[0].(map[string]any); ok {
			if parts, ok := row["metadataParts"].([]any); ok && len(parts) > 0 {
				if part, ok := parts[0].(map[string]any); ok {
					channel = text(part["text"])
				}
			}
		}
	}
	if isChannelMissing(channel) {
		for _, value := range texts {
			if !isChannelMissing(value) {
				channel = value
				break
			}
		}
	}
	return channel, published
}

func overlayDuration(value map[string]any) string {
	if valueText := text(value["text"]); valueText != "" {
		return valueText
	}
	return text(value)
}

func accessibilityDuration(value string) string {
	lower := strings.ToLower(value)
	index := -1
	for _, marker := range []string{", hour", ", minute", ", second"} {
		if candidate := strings.Index(lower, marker); candidate >= 0 && (index < 0 || candidate < index) {
			index = candidate
		}
	}
	if index < 0 {
		return ""
	}
	duration := strings.TrimSpace(value[index+2:])
	for _, character := range duration {
		if character >= '0' && character <= '9' {
			return duration
		}
	}
	return ""
}

func lockupDuration(value any) string {
	switch item := value.(type) {
	case map[string]any:
		for _, key := range []string{"thumbnailOverlayTimeStatusViewModel", "thumbnailOverlayTimeStatusRenderer", "timeStatus"} {
			if status, ok := item[key].(map[string]any); ok {
				if duration := overlayDuration(status); duration != "" {
					return duration
				}
			}
		}
		if accessibility, ok := item["accessibilityContext"].(map[string]any); ok {
			if label, ok := accessibility["label"].(string); ok {
				if duration := accessibilityDuration(label); duration != "" {
					return duration
				}
			}
		}
		for _, child := range item {
			if duration := lockupDuration(child); duration != "" {
				return duration
			}
		}
	case []any:
		for _, child := range item {
			if duration := lockupDuration(child); duration != "" {
				return duration
			}
		}
	}
	return ""
}

func initialData(html string) (any, bool) {
	marker := "var ytInitialData = "
	start := strings.Index(html, marker)
	if start < 0 {
		return nil, false
	}
	jsonText := html[start+len(marker):]
	first := strings.IndexByte(jsonText, '{')
	if first < 0 {
		return nil, false
	}
	depth := 0
	quote := false
	escaped := false
	for index := first; index < len(jsonText); index++ {
		character := jsonText[index]
		if quote {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				quote = false
			}
			continue
		}
		switch character {
		case '"':
			quote = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				var value any
				if err := json.Unmarshal([]byte(jsonText[first:index+1]), &value); err != nil {
					return nil, false
				}
				return value, true
			}
		}
	}
	return nil, false
}

func configValue(html, name string) (string, bool) {
	marker := fmt.Sprintf("\"%s\":\"", name)
	start := strings.Index(html, marker)
	if start < 0 {
		return "", false
	}
	start += len(marker)
	end := strings.IndexByte(html[start:], '"')
	if end < 0 {
		return "", false
	}
	return html[start : start+end], true
}

func continuation(value any) (string, bool) {
	switch item := value.(type) {
	case map[string]any:
		if command, ok := item["continuationCommand"].(map[string]any); ok {
			if token, ok := command["token"].(string); ok {
				return token, true
			}
		}
		for _, child := range item {
			if token, ok := continuation(child); ok {
				return token, true
			}
		}
	case []any:
		for _, child := range item {
			if token, ok := continuation(child); ok {
				return token, true
			}
		}
	}
	return "", false
}

func browseID(value any) string {
	switch item := value.(type) {
	case map[string]any:
		if navigation, ok := item["navigationEndpoint"].(map[string]any); ok {
			if browse, ok := navigation["browseEndpoint"].(map[string]any); ok {
				if id, ok := browse["browseId"].(string); ok {
					return id
				}
			}
		}
		for _, child := range item {
			if id := browseID(child); id != "" {
				return id
			}
		}
	case []any:
		for _, child := range item {
			if id := browseID(child); id != "" {
				return id
			}
		}
	}
	return ""
}

func mapValue(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func firstThumbnail(value any) string {
	item := mapValue(value)
	thumbnails, _ := item["thumbnails"].([]any)
	if len(thumbnails) == 0 {
		return ""
	}
	return stringValue(mapValue(thumbnails[0])["url"])
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func resultFromRenderer(id string, title, channel, duration, thumbnail, published any) SearchResult {
	return SearchResult{ID: id, Title: text(title), Channel: text(channel), ChannelID: browseID(channel), Duration: text(duration), Thumbnail: firstThumbnail(thumbnail), Published: text(published)}
}

func collectResults(value any, results *[]SearchResult, seen map[string]struct{}) {
	if len(*results) >= 30 {
		return
	}
	item := mapValue(value)
	if renderer, ok := item["videoRenderer"].(map[string]any); ok {
		id := stringValue(renderer["videoId"])
		if id != "" {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				channel := firstNonNil(renderer["ownerText"], renderer["shortBylineText"], renderer["longBylineText"], renderer["bylineText"])
				published := firstNonNil(renderer["publishedTimeText"], renderer["publishedText"])
				*results = append(*results, resultFromRenderer(id, renderer["title"], channel, renderer["lengthText"], renderer["thumbnail"], published))
			}
		}
	}
	if renderer, ok := item["reelItemRenderer"].(map[string]any); ok {
		id := stringValue(renderer["videoId"])
		if id != "" {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				channel := firstNonNil(renderer["ownerText"], renderer["shortBylineText"], renderer["longBylineText"], renderer["bylineText"])
				published := firstNonNil(renderer["publishedTimeText"], renderer["publishedText"])
				result := resultFromRenderer(id, firstNonNil(renderer["headline"], renderer["title"]), channel, renderer["lengthText"], renderer["thumbnail"], published)
				result.IsShort = true
				*results = append(*results, result)
			}
		}
	}
	if renderer, ok := item["gridVideoRenderer"].(map[string]any); ok {
		id := stringValue(renderer["videoId"])
		if id != "" {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				channel := firstNonNil(renderer["ownerText"], renderer["shortBylineText"], renderer["longBylineText"], renderer["bylineText"])
				published := firstNonNil(renderer["publishedTimeText"], renderer["publishedText"])
				*results = append(*results, resultFromRenderer(id, renderer["title"], channel, renderer["lengthText"], renderer["thumbnail"], published))
			}
		}
	}
	if lockup, ok := item["lockupViewModel"].(map[string]any); ok {
		thumbnail := ""
		if content, ok := lockup["contentImage"].(map[string]any); ok {
			if model, ok := content["thumbnailViewModel"].(map[string]any); ok {
				if image, ok := model["image"].(map[string]any); ok {
					if sources, ok := image["sources"].([]any); ok && len(sources) > 0 {
						thumbnail = stringValue(mapValue(sources[0])["url"])
					}
				}
			}
		}
		id := ""
		if parts := strings.SplitN(thumbnail, "/vi/", 2); len(parts) == 2 {
			id = strings.SplitN(parts[1], "/", 2)[0]
		}
		if id != "" {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				metadata := mapValue(lockup["metadata"])
				metadata = mapValue(metadata["lockupMetadataViewModel"])
				channel, published := lockupMetadata(lockup)
				*results = append(*results, SearchResult{ID: id, Title: text(metadata["title"]), Channel: channel, ChannelID: browseID(lockup), Duration: lockupDuration(lockup), Thumbnail: thumbnail, Published: published})
			}
		}
	}
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			collectResults(child, results, seen)
		}
	case map[string]any:
		for _, child := range item {
			collectResults(child, results, seen)
		}
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func collectReelResults(html string, results *[]SearchResult, seen map[string]struct{}) {
	markers := []string{"\"reelWatchEndpoint\":{\"videoId\":\"", `\"reelWatchEndpoint\":{\"videoId\":\"`, `reelWatchEndpoint\x22:\x7b\x22videoId\x22:\x22`}
	for _, marker := range markers {
		remainder := html
		for len(*results) < 30 {
			index := strings.Index(remainder, marker)
			if index < 0 {
				break
			}
			value := remainder[index+len(marker):]
			length := 0
			for length < len(value) {
				character := value[length]
				if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' || character == '-' {
					length++
					continue
				}
				break
			}
			id := value[:length]
			if id != "" {
				if _, exists := seen[id]; !exists {
					seen[id] = struct{}{}
					*results = append(*results, SearchResult{ID: id, Title: "YouTube Short", Thumbnail: fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", id), IsShort: true})
				}
			}
			remainder = value[length:]
		}
	}
}

func newRequest(method, endpoint string, query url.Values, body any) (*http.Request, error) {
	if len(query) > 0 {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		parsed.RawQuery = query.Encode()
		endpoint = parsed.String()
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", userAgent)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return request, nil
}

func doRequest(request *http.Request) ([]byte, error) {
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return nil, readErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP %s", response.Status)
	}
	return data, nil
}

func getText(endpoint string, query url.Values, failure string) (string, error) {
	request, err := newRequest(http.MethodGet, endpoint, query, nil)
	if err != nil {
		return "", err
	}
	data, err := doRequest(request)
	if err != nil {
		return "", fmt.Errorf("%s: %w", failure, err)
	}
	return string(data), nil
}

func postJSON(endpoint string, query url.Values, body any, headers map[string]string, failure string) (any, error) {
	request, err := newRequest(http.MethodPost, endpoint, query, body)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	data, err := doRequest(request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", failure, err)
	}
	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *YtLiteService) searchYouTubeSync(query string, state *searchState) (SearchPage, error) {
	page, err := getText("https://www.youtube.com/results", url.Values{"search_query": []string{query}, "hl": []string{"en"}, "gl": []string{"US"}}, "YouTube search request failed")
	if err != nil {
		return SearchPage{}, err
	}
	data, ok := initialData(page)
	if !ok {
		return SearchPage{}, errors.New("YouTube did not return search data")
	}
	results := make([]SearchResult, 0, 30)
	collectResults(data, &results, make(map[string]struct{}))
	token, hasToken := continuation(data)
	apiKey, hasAPIKey := configValue(page, "INNERTUBE_API_KEY")
	clientVersion, hasClientVersion := configValue(page, "INNERTUBE_CONTEXT_CLIENT_VERSION")
	var cursor *string
	if hasToken && hasAPIKey && hasClientVersion {
		key := strconv.FormatUint(atomic.AddUint64(&nextCursor, 1), 10)
		visitor, _ := configValue(page, "VISITOR_DATA")
		state.mu.Lock()
		clear(state.cursors)
		state.cursors[key] = searchCursor{Token: token, APIKey: apiKey, ClientVersion: clientVersion, VisitorData: visitor}
		state.mu.Unlock()
		cursor = &key
	}
	return SearchPage{Results: results, Cursor: cursor}, nil
}

func (s *YtLiteService) searchYouTubeMoreSync(cursor string, state *searchState) (SearchPage, error) {
	state.mu.Lock()
	current, ok := state.cursors[cursor]
	if ok {
		delete(state.cursors, cursor)
	}
	state.mu.Unlock()
	if !ok {
		return SearchPage{}, errors.New("Search has no more results")
	}
	body := map[string]any{"context": map[string]any{"client": map[string]any{"clientName": "WEB", "clientVersion": current.ClientVersion, "visitorData": current.VisitorData}}, "continuation": current.Token}
	response, err := postJSON(fmt.Sprintf("https://www.youtube.com/youtubei/v1/search?prettyPrint=false&key=%s", url.QueryEscape(current.APIKey)), nil, body, map[string]string{"X-YouTube-Client-Name": "1", "X-YouTube-Client-Version": current.ClientVersion}, "YouTube continuation request failed")
	if err != nil {
		return SearchPage{}, err
	}
	results := make([]SearchResult, 0, 30)
	collectResults(response, &results, make(map[string]struct{}))
	if token, ok := continuation(response); ok {
		state.mu.Lock()
		state.cursors[cursor] = searchCursor{Token: token, APIKey: current.APIKey, ClientVersion: current.ClientVersion, VisitorData: current.VisitorData}
		state.mu.Unlock()
		return SearchPage{Results: results, Cursor: &cursor}, nil
	}
	return SearchPage{Results: results}, nil
}

func pageFromBrowse(response any) SearchPage {
	results := make([]SearchResult, 0, 30)
	collectResults(response, &results, make(map[string]struct{}))
	return SearchPage{Results: results}
}

func channelTabSync(channelID, title string) (SearchPage, error) {
	page, err := getText(fmt.Sprintf("https://www.youtube.com/channel/%s/%s", channelID, strings.ToLower(title)), url.Values{"hl": []string{"en"}, "gl": []string{"US"}}, "YouTube channel request failed")
	if err != nil {
		return SearchPage{}, err
	}
	data, ok := initialData(page)
	if !ok {
		return SearchPage{}, errors.New("YouTube did not return channel data")
	}
	return pageFromBrowse(data), nil
}

func subscriptionShortsSync(subscriptions []Subscription) (SearchPage, []Subscription, error) {
	results := make([]SearchResult, 0, 30)
	seen := make(map[string]struct{})
	for index := range subscriptions {
		subscription := &subscriptions[index]
		if subscription.ChannelID == "" {
			channelID, err := channelIDSync(subscription.Channel)
			if err != nil {
				return SearchPage{}, subscriptions, err
			}
			subscription.ChannelID = channelID
		}
		page, err := getText(fmt.Sprintf("https://www.youtube.com/channel/%s/shorts", subscription.ChannelID), url.Values{"hl": []string{"en"}, "gl": []string{"US"}}, "YouTube Shorts request failed")
		if err != nil {
			return SearchPage{}, subscriptions, err
		}
		data, ok := initialData(page)
		if !ok {
			return SearchPage{}, subscriptions, errors.New("YouTube did not return Shorts data")
		}
		collectResults(data, &results, seen)
		if len(results) >= 30 {
			break
		}
	}
	return SearchPage{Results: results}, subscriptions, nil
}

func shortsSync() (SearchPage, error) {
	page, err := getText("https://www.youtube.com/shorts/", url.Values{"hl": []string{"en"}, "gl": []string{"US"}}, "YouTube Shorts request failed")
	if err != nil {
		return SearchPage{}, err
	}
	results := make([]SearchResult, 0, 30)
	seen := make(map[string]struct{})
	if data, ok := initialData(page); ok {
		collectResults(data, &results, seen)
	}
	collectReelResults(page, &results, seen)
	return SearchPage{Results: results}, nil
}

func shortsMoreSync(videoID string) (SearchPage, error) {
	page, err := getText(fmt.Sprintf("https://www.youtube.com/shorts/%s", videoID), url.Values{"hl": []string{"en"}, "gl": []string{"US"}}, "YouTube Shorts request failed")
	if err != nil {
		return SearchPage{}, err
	}
	results := make([]SearchResult, 0, 30)
	collectReelResults(page, &results, make(map[string]struct{}))
	return SearchPage{Results: results}, nil
}

func (s *YtLiteService) subscriptionVideosSync(subscriptions []Subscription) (SearchPage, []Subscription, error) {
	results := make([]SearchResult, 0, 30)
	seen := make(map[string]struct{})
	for index := range subscriptions {
		subscription := &subscriptions[index]
		if subscription.ChannelID == "" {
			channelID, err := channelIDSync(subscription.Channel)
			if err != nil {
				return SearchPage{}, subscriptions, err
			}
			subscription.ChannelID = channelID
		}
		page, err := getText(fmt.Sprintf("https://www.youtube.com/channel/%s/videos", subscription.ChannelID), url.Values{"hl": []string{"en"}, "gl": []string{"US"}}, "YouTube Home request failed")
		if err != nil {
			return SearchPage{}, subscriptions, err
		}
		data, ok := initialData(page)
		if !ok {
			return SearchPage{}, subscriptions, errors.New("YouTube did not return Home data")
		}
		start := len(results)
		collectResults(data, &results, seen)
		fillChannel(results[start:], subscription.Channel, subscription.ChannelID)
		if len(results) >= 30 {
			break
		}
	}
	if len(results) == 0 {
		page, err := s.searchYouTubeSync("music", newSearchState())
		return page, subscriptions, err
	}
	return SearchPage{Results: results}, subscriptions, nil
}

func channelIDSync(channel string) (string, error) {
	page, err := getText("https://www.youtube.com/results", url.Values{"search_query": []string{channel}, "hl": []string{"en"}, "gl": []string{"US"}}, "YouTube channel lookup failed")
	if err != nil {
		return "", err
	}
	data, ok := initialData(page)
	if !ok {
		return "", errors.New("YouTube did not return channel lookup data")
	}
	results := make([]SearchResult, 0, 30)
	collectResults(data, &results, make(map[string]struct{}))
	for _, result := range results {
		if strings.EqualFold(result.Channel, channel) && result.ChannelID != "" {
			return result.ChannelID, nil
		}
	}
	for _, result := range results {
		if result.ChannelID != "" {
			return result.ChannelID, nil
		}
	}
	return "", errors.New("Could not find that channel's YouTube ID")
}

func channelAvatarSync(channelID string) (string, error) {
	page, err := getText(fmt.Sprintf("https://www.youtube.com/channel/%s", channelID), url.Values{"hl": []string{"en"}, "gl": []string{"US"}}, "YouTube channel request failed")
	if err != nil {
		return "", err
	}
	fromMeta := ""
	if index := strings.Index(page, "property=\"og:image\""); index >= 0 {
		if offset := strings.Index(page[index:], "content=\""); offset >= 0 {
			value := page[index+offset+9:]
			if end := strings.IndexByte(value, '"'); end >= 0 {
				fromMeta = value[:end]
			}
		}
	}
	fromImage := ""
	for _, marker := range []string{"https://yt3.ggpht.com/", "https://yt3.googleusercontent.com/"} {
		if index := strings.Index(page, marker); index >= 0 {
			value := page[index:]
			if end := strings.IndexByte(value, '"'); end >= 0 {
				fromImage = value[:end]
				break
			}
		}
	}
	if fromMeta == "" {
		fromMeta = fromImage
	}
	if fromMeta == "" {
		return "", errors.New("YouTube did not return a channel avatar")
	}
	return strings.ReplaceAll(fromMeta, `\u0026`, "&"), nil
}

func (s *YtLiteService) SearchYouTube(query string) (SearchPage, error) {
	return s.searchYouTubeSync(query, s.state)
}

func (s *YtLiteService) SearchYouTubeMore(cursor string) (SearchPage, error) {
	return s.searchYouTubeMoreSync(cursor, s.state)
}

func (s *YtLiteService) LoadShorts() (SearchPage, error) {
	page, err := shortsSync()
	if err != nil {
		return SearchPage{}, err
	}
	if len(page.Results) > 0 {
		return page, nil
	}
	s.storage.Lock()
	subscriptions, err := readSubscriptions()
	s.storage.Unlock()
	if err != nil {
		return SearchPage{}, err
	}
	page, subscriptions, err = subscriptionShortsSync(subscriptions)
	if err != nil {
		return SearchPage{}, err
	}
	s.storage.Lock()
	path, pathErr := subscriptionsPath()
	if pathErr == nil {
		pathErr = writeJSON(path, subscriptions)
	}
	s.storage.Unlock()
	return page, pathErr
}

func (s *YtLiteService) LoadShortsMore(videoID string) (SearchPage, error) {
	return shortsMoreSync(videoID)
}

func (s *YtLiteService) LoadHome() (SearchPage, error) {
	s.storage.Lock()
	subscriptions, err := readSubscriptions()
	s.storage.Unlock()
	if err != nil {
		return SearchPage{}, err
	}
	page, subscriptions, err := s.subscriptionVideosSync(subscriptions)
	if err != nil {
		return SearchPage{}, err
	}
	s.storage.Lock()
	path, pathErr := subscriptionsPath()
	if pathErr == nil {
		pathErr = writeJSON(path, subscriptions)
	}
	s.storage.Unlock()
	return page, pathErr
}

func (s *YtLiteService) LoadChannelVideos(channel, channelID string) (SearchPage, error) {
	resolvedID := channelID
	if resolvedID == "" {
		var err error
		resolvedID, err = channelIDSync(channel)
		if err != nil {
			return SearchPage{}, err
		}
	}
	page, err := channelTabSync(resolvedID, "Videos")
	if err != nil {
		return SearchPage{}, err
	}
	fillChannel(page.Results, channel, resolvedID)
	s.storage.Lock()
	subscriptions, subscriptionsErr := readSubscriptions()
	if subscriptionsErr == nil {
		for index := range subscriptions {
			if strings.EqualFold(subscriptions[index].Channel, channel) {
				subscriptions[index].ChannelID = resolvedID
			}
		}
		path, pathErr := subscriptionsPath()
		if pathErr == nil {
			subscriptionsErr = writeJSON(path, subscriptions)
		} else {
			subscriptionsErr = pathErr
		}
	}
	s.storage.Unlock()
	return page, subscriptionsErr
}

func (s *YtLiteService) LoadSubscriptionAvatar(channel, channelID string) (string, error) {
	resolvedID := channelID
	if resolvedID == "" {
		var err error
		resolvedID, err = channelIDSync(channel)
		if err != nil {
			return "", err
		}
	}
	avatar, err := channelAvatarSync(resolvedID)
	if err != nil {
		return "", err
	}
	s.storage.Lock()
	subscriptions, subscriptionsErr := readSubscriptions()
	if subscriptionsErr == nil {
		for index := range subscriptions {
			if strings.EqualFold(subscriptions[index].Channel, channel) {
				subscriptions[index].ChannelID = resolvedID
				subscriptions[index].Avatar = avatar
			}
		}
		path, pathErr := subscriptionsPath()
		if pathErr == nil {
			subscriptionsErr = writeJSON(path, subscriptions)
		} else {
			subscriptionsErr = pathErr
		}
	}
	s.storage.Unlock()
	if subscriptionsErr != nil {
		return "", subscriptionsErr
	}
	return avatar, nil
}
