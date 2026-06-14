package glance

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ConfigManager handles YAML AST mutations on glance.yml while preserving formatting and comments.
type ConfigManager struct {
	configPath   string
	configFileMu *sync.Mutex
	reloadFn     func() error
}

// NewConfigManager creates a new ConfigManager.
func NewConfigManager(configPath string, configFileMu *sync.Mutex, reloadFn func() error) *ConfigManager {
	return &ConfigManager{
		configPath:   configPath,
		configFileMu: configFileMu,
		reloadFn:     reloadFn,
	}
}

// ReadAST reads the configuration file and returns the root yaml.Node AST.
func (cm *ConfigManager) ReadAST() (yaml.Node, error) {
	var rootNode yaml.Node
	configBytes, err := os.ReadFile(cm.configPath)
	if err != nil {
		return rootNode, err
	}
	if err := yaml.Unmarshal(configBytes, &rootNode); err != nil {
		return rootNode, err
	}
	return rootNode, nil
}

// AddPage appends a new page to the pages sequence block.
func (cm *ConfigManager) AddPage(name string) error {
	cm.configFileMu.Lock()
	defer cm.configFileMu.Unlock()

	rootNode, err := cm.ReadAST()
	if err != nil {
		return err
	}

	if len(rootNode.Content) == 0 {
		return fmt.Errorf("empty YAML document")
	}
	rootMap := rootNode.Content[0]
	pagesNode := findMapValue(rootMap, "pages")
	if pagesNode == nil || pagesNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("pages block not found")
	}

	newPageMap := map[string]interface{}{
		"name": name,
		"columns": []map[string]interface{}{
			{
				"size":    "full",
				"widgets": []interface{}{},
			},
		},
	}

	var newPageDoc yaml.Node
	if err := newPageDoc.Encode(newPageMap); err != nil {
		return err
	}

	var newPageNode *yaml.Node
	if newPageDoc.Kind == yaml.DocumentNode && len(newPageDoc.Content) > 0 {
		newPageNode = newPageDoc.Content[0]
	} else {
		newPageNode = &newPageDoc
	}

	pagesNode.Content = append(pagesNode.Content, newPageNode)

	if err := validateASTConfig(&rootNode); err != nil {
		return err
	}

	if err := saveNodeToDisk(cm.configPath, &rootNode); err != nil {
		return err
	}

	return cm.reloadFn()
}

// DeletePage deletes a page by its slug from the pages sequence block.
func (cm *ConfigManager) DeletePage(slug string) error {
	cm.configFileMu.Lock()
	defer cm.configFileMu.Unlock()

	rootNode, err := cm.ReadAST()
	if err != nil {
		return err
	}

	if len(rootNode.Content) == 0 {
		return fmt.Errorf("empty YAML document")
	}
	rootMap := rootNode.Content[0]
	pagesNode := findMapValue(rootMap, "pages")
	if pagesNode == nil || pagesNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("pages block not found")
	}

	if len(pagesNode.Content) <= 1 {
		return fmt.Errorf("cannot delete the last page")
	}

	found := -1
	for i := 0; i < len(pagesNode.Content); i++ {
		pageNode := pagesNode.Content[i]
		nameNode := findMapValue(pageNode, "name")
		titleNode := findMapValue(pageNode, "title")
		slugNode := findMapValue(pageNode, "slug")

		currSlug := ""
		if slugNode != nil {
			currSlug = slugNode.Value
		} else if nameNode != nil {
			currSlug = titleToSlug(nameNode.Value)
		} else if titleNode != nil {
			currSlug = titleToSlug(titleNode.Value)
		}

		if currSlug == slug {
			found = i
			break
		}
	}

	if found == -1 {
		return fmt.Errorf("page not found")
	}

	pagesNode.Content = append(pagesNode.Content[:found], pagesNode.Content[found+1:]...)

	if err := validateASTConfig(&rootNode); err != nil {
		return err
	}

	if err := saveNodeToDisk(cm.configPath, &rootNode); err != nil {
		return err
	}

	return cm.reloadFn()
}

// ImportConfig overwrites the configuration file with raw YAML bytes.
func (cm *ConfigManager) ImportConfig(contentBytes []byte) error {
	cm.configFileMu.Lock()
	defer cm.configFileMu.Unlock()

	var testConfig Config
	if err := yaml.Unmarshal(contentBytes, &testConfig); err != nil {
		return fmt.Errorf("invalid YAML syntax: %w", err)
	}

	if err := configIsValid(&testConfig); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	if err := os.WriteFile(cm.configPath, contentBytes, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return cm.reloadFn()
}

// SaveLayout updates widget positioning and ordering on a page.
func (cm *ConfigManager) SaveLayout(pageSlug string, head []string, columns [][]string, columnSizes []string) error {
	cm.configFileMu.Lock()
	defer cm.configFileMu.Unlock()

	rootNode, err := cm.ReadAST()
	if err != nil {
		return err
	}

	pageNode, err := findPageNode(&rootNode, pageSlug)
	if err != nil {
		return err
	}

	columnsNode := findMapValue(pageNode, "columns")
	if columnsNode == nil || columnsNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("columns not found on page")
	}

	// Catalog all original widgets across the page to re-map them by ID
	originalNodesMap := make(map[string]*yaml.Node)
	nodesToDelete := make(map[*yaml.Node]bool)

	// Catalog head widgets
	hNode := findMapValue(pageNode, "head-widgets")
	if hNode != nil && hNode.Kind == yaml.SequenceNode {
		for wIdx, w := range hNode.Content {
			wID := pageSlug + "/head/" + strconv.Itoa(wIdx)
			originalNodesMap[wID] = w
			nodesToDelete[w] = true
		}
	}

	// Catalog columns widgets
	for cIdx, colNode := range columnsNode.Content {
		colWidgets := findMapValue(colNode, "widgets")
		if colWidgets != nil && colWidgets.Kind == yaml.SequenceNode {
			for wIdx, w := range colWidgets.Content {
				wID := pageSlug + "/" + strconv.Itoa(cIdx) + "/" + strconv.Itoa(wIdx)
				originalNodesMap[wID] = w
				nodesToDelete[w] = true
			}
		}
	}

	// 1. Rebuild Head Widgets sequence
	var newHeadContent []*yaml.Node
	for _, wID := range head {
		if node, exists := originalNodesMap[wID]; exists {
			newHeadContent = append(newHeadContent, node)
			delete(nodesToDelete, node)
		}
	}

	if len(newHeadContent) > 0 {
		hNode = findMapValue(pageNode, "head-widgets")
		if hNode == nil {
			hNode = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			pageNode.Content = append(pageNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "head-widgets"},
				hNode,
			)
		}
		hNode.Content = newHeadContent
		hNode.Style = 0
	} else {
		// Remove head-widgets node if empty
		removeMapKey(pageNode, "head-widgets")
	}

	// 2. Rebuild Columns
	var newColumns []*yaml.Node
	for colIdx, colWidgetsIDs := range columns {
		var colNode *yaml.Node
		if colIdx < len(columnsNode.Content) {
			colNode = columnsNode.Content[colIdx]
		} else {
			colNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}

		// Update column size
		size := "full"
		if colIdx < len(columnSizes) {
			size = columnSizes[colIdx]
		}
		updateMapValue(colNode, "size", size)

		var newColWidgets []*yaml.Node
		for _, wID := range colWidgetsIDs {
			if node, exists := originalNodesMap[wID]; exists {
				newColWidgets = append(newColWidgets, node)
				delete(nodesToDelete, node)
			}
		}

		colWidgetsNode := findMapValue(colNode, "widgets")
		if colWidgetsNode == nil {
			colWidgetsNode = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			colNode.Content = append(colNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "widgets"},
				colWidgetsNode,
			)
		}
		colWidgetsNode.Content = newColWidgets
		colWidgetsNode.Style = 0
		newColumns = append(newColumns, colNode)
	}
	columnsNode.Content = newColumns

	if err := validateASTConfig(&rootNode); err != nil {
		return err
	}

	if err := saveNodeToDisk(cm.configPath, &rootNode); err != nil {
		return err
	}

	return cm.reloadFn()
}

// AddWidget appends a new widget to a page's column or head block.
func (cm *ConfigManager) AddWidget(pageSlug string, columnIndex string, widgetType string, properties map[string]interface{}) error {
	cm.configFileMu.Lock()
	defer cm.configFileMu.Unlock()

	rootNode, err := cm.ReadAST()
	if err != nil {
		return err
	}

	if len(rootNode.Content) == 0 {
		return fmt.Errorf("empty YAML document")
	}
	rootMap := rootNode.Content[0]

	// Process Spotify credentials
	if widgetType == "spotify" && properties != nil {
		var clientID, clientSecret, redirectURL, accessToken, refreshToken string
		if cid, ok := properties["client_id"].(string); ok {
			clientID = strings.TrimSpace(cid)
		}
		if csec, ok := properties["client_secret"].(string); ok {
			clientSecret = strings.TrimSpace(csec)
		}
		if rurl, ok := properties["redirect_url"].(string); ok {
			redirectURL = strings.TrimSpace(rurl)
		}
		if at, ok := properties["access_token"].(string); ok {
			accessToken = strings.TrimSpace(at)
		}
		if rt, ok := properties["refresh_token"].(string); ok {
			refreshToken = strings.TrimSpace(rt)
		}

		delete(properties, "client_id")
		delete(properties, "client_secret")
		delete(properties, "redirect_url")
		delete(properties, "access_token")
		delete(properties, "refresh_token")

		if accessToken != "" && accessToken != "********" {
			_ = Store.SetSetting("spotify_access_token", accessToken)
		}
		if refreshToken != "" && refreshToken != "********" {
			_ = Store.SetSetting("spotify_refresh_token", refreshToken)
		}
		if (accessToken != "" && accessToken != "********") || (refreshToken != "" && refreshToken != "********") {
			_ = Store.SetSetting("spotify_authorized", "true")
		}

		if clientID != "" || (clientSecret != "" && clientSecret != "********") || redirectURL != "" {
			spotifyNode := findMapValue(rootMap, "spotify")
			if spotifyNode == nil || spotifyNode.Kind != yaml.MappingNode {
				spotifyNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "spotify"}
				rootMap.Content = append(rootMap.Content, keyNode, spotifyNode)
			}
			if clientID != "" {
				updateMapValue(spotifyNode, "client-id", clientID)
			}
			if clientSecret != "" && clientSecret != "********" {
				updateMapValue(spotifyNode, "client-secret", clientSecret)
			}
			if redirectURL != "" {
				updateMapValue(spotifyNode, "redirect-url", redirectURL)
			}
		}
	}

	// Process Calendar/Gmail credentials
	if (widgetType == "calendar" || widgetType == "gmail") && properties != nil {
		var clientID, clientSecret, redirectURL string
		if cid, ok := properties["google_client_id"].(string); ok {
			clientID = strings.TrimSpace(cid)
		}
		if csec, ok := properties["google_client_secret"].(string); ok {
			clientSecret = strings.TrimSpace(csec)
		}
		if rurl, ok := properties["google_redirect_url"].(string); ok {
			redirectURL = strings.TrimSpace(rurl)
		}

		delete(properties, "google_client_id")
		delete(properties, "google_client_secret")
		delete(properties, "google_redirect_url")

		if clientID != "" || (clientSecret != "" && clientSecret != "********") || redirectURL != "" {
			googleNode := findMapValue(rootMap, "google")
			if googleNode == nil || googleNode.Kind != yaml.MappingNode {
				googleNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "google"}
				rootMap.Content = append(rootMap.Content, keyNode, googleNode)
			}
			if clientID != "" {
				updateMapValue(googleNode, "client-id", clientID)
			}
			if clientSecret != "" && clientSecret != "********" {
				updateMapValue(googleNode, "client-secret", clientSecret)
			}
			if redirectURL != "" {
				updateMapValue(googleNode, "redirect-url", redirectURL)
			}
		}
	}

	// Process Hue credentials
	if widgetType == "hue" && properties != nil {
		var clientID, clientSecret, redirectURL string
		if cid, ok := properties["hue_client_id"].(string); ok {
			clientID = strings.TrimSpace(cid)
		}
		if csec, ok := properties["hue_client_secret"].(string); ok {
			clientSecret = strings.TrimSpace(csec)
		}
		if rurl, ok := properties["hue_redirect_url"].(string); ok {
			redirectURL = strings.TrimSpace(rurl)
		}

		delete(properties, "hue_client_id")
		delete(properties, "hue_client_secret")
		delete(properties, "hue_redirect_url")

		if clientID != "" || (clientSecret != "" && clientSecret != "********") || redirectURL != "" {
			hueNode := findMapValue(rootMap, "hue")
			if hueNode == nil || hueNode.Kind != yaml.MappingNode {
				hueNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "hue"}
				rootMap.Content = append(rootMap.Content, keyNode, hueNode)
			}
			if clientID != "" {
				updateMapValue(hueNode, "client-id", clientID)
			}
			if clientSecret != "" && clientSecret != "********" {
				updateMapValue(hueNode, "client-secret", clientSecret)
			}
			if redirectURL != "" {
				updateMapValue(hueNode, "redirect-url", redirectURL)
			}
		}
	}

	pageNode, err := findPageNode(&rootNode, pageSlug)
	if err != nil {
		return err
	}

	var widgetsNode *yaml.Node

	if columnIndex == "head" {
		widgetsNode = findMapValue(pageNode, "head-widgets")
		if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode {
			headKeyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "head-widgets"}
			widgetsNode = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			pageNode.Content = append(pageNode.Content, headKeyNode, widgetsNode)
		}
	} else {
		columnsNode := findMapValue(pageNode, "columns")
		if columnsNode == nil || columnsNode.Kind != yaml.SequenceNode {
			return fmt.Errorf("columns not found")
		}

		colIdx, err := strconv.Atoi(columnIndex)
		if err != nil || colIdx < 0 || colIdx >= len(columnsNode.Content) {
			return fmt.Errorf("invalid column index")
		}

		colNode := columnsNode.Content[colIdx]
		widgetsNode = findMapValue(colNode, "widgets")
		if widgetsNode == nil {
			widgetsNode = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			colNode.Content = append(colNode.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "widgets"}, widgetsNode)
		}
	}

	// Clean up properties to avoid string representation issues
	widgetMap := map[string]interface{}{
		"type": widgetType,
	}
	if widgetType == "group" {
		widgetMap["widgets"] = []interface{}{}
	}
	for k, v := range properties {
		if f, ok := v.(float64); ok && f == float64(int(f)) {
			switch k {
			case "update-interval", "height", "limit", "collapse-after", "thumbnail-height", "card-height", "viewport-limit", "max-days-ahead", "commits-limit", "collapse-after-rows", "pull-requests-limit", "issues-limit":
				v = int(f)
			}
		}
		if s, ok := v.(string); ok {
			if n, err := strconv.Atoi(s); err == nil {
				switch k {
				case "update-interval", "height", "limit", "collapse-after", "thumbnail-height", "card-height", "viewport-limit", "max-days-ahead", "commits-limit", "collapse-after-rows", "pull-requests-limit", "issues-limit":
					v = n
				}
			}
		}
		widgetMap[k] = v
	}

	newWidgetNode := &yaml.Node{}
	if err := newWidgetNode.Encode(widgetMap); err != nil {
		return err
	}

	widgetsNode.Style = 0
	widgetsNode.Content = append(widgetsNode.Content, newWidgetNode)

	if err := validateASTConfig(&rootNode); err != nil {
		return err
	}

	if err := saveNodeToDisk(cm.configPath, &rootNode); err != nil {
		return err
	}

	return cm.reloadFn()
}

// UpdateWidget updates properties of an existing widget in place.
func (cm *ConfigManager) UpdateWidget(pageSlug string, columnIndex string, widgetIndex int, properties map[string]interface{}) error {
	cm.configFileMu.Lock()
	defer cm.configFileMu.Unlock()

	rootNode, err := cm.ReadAST()
	if err != nil {
		return err
	}

	if len(rootNode.Content) == 0 {
		return fmt.Errorf("empty YAML document")
	}
	rootMap := rootNode.Content[0]

	pageNode, err := findPageNode(&rootNode, pageSlug)
	if err != nil {
		return err
	}

	var widgetsNode *yaml.Node

	if columnIndex == "head" {
		widgetsNode = findMapValue(pageNode, "head-widgets")
		if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode {
			return fmt.Errorf("head-widgets not found")
		}
	} else if strings.Contains(columnIndex, ":") {
		// Group nested widget update
		parts := strings.Split(columnIndex, ":")
		origColStr := parts[0]
		origIdx, _ := strconv.Atoi(parts[1])

		var baseNode *yaml.Node
		if origColStr == "head" {
			headNode := findMapValue(pageNode, "head-widgets")
			if headNode != nil && headNode.Kind == yaml.SequenceNode && origIdx >= 0 && origIdx < len(headNode.Content) {
				baseNode = headNode.Content[origIdx]
			}
		} else {
			columnsNode := findMapValue(pageNode, "columns")
			colIdx, err := strconv.Atoi(origColStr)
			if err == nil && columnsNode != nil && columnsNode.Kind == yaml.SequenceNode && colIdx >= 0 && colIdx < len(columnsNode.Content) {
				colNode := columnsNode.Content[colIdx]
				colWidgets := findMapValue(colNode, "widgets")
				if colWidgets != nil && colWidgets.Kind == yaml.SequenceNode && origIdx >= 0 && origIdx < len(colWidgets.Content) {
					baseNode = colWidgets.Content[origIdx]
				}
			}
		}

		if baseNode == nil {
			return fmt.Errorf("group widget not found")
		}

		widgetsNode = findMapValue(baseNode, "widgets")
		if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode {
			return fmt.Errorf("nested widgets sequence not found in group")
		}
	} else {
		columnsNode := findMapValue(pageNode, "columns")
		if columnsNode == nil || columnsNode.Kind != yaml.SequenceNode {
			return fmt.Errorf("columns not found")
		}

		colIdx, err := strconv.Atoi(columnIndex)
		if err != nil || colIdx < 0 || colIdx >= len(columnsNode.Content) {
			return fmt.Errorf("invalid column index")
		}

		colNode := columnsNode.Content[colIdx]
		widgetsNode = findMapValue(colNode, "widgets")
		if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode {
			return fmt.Errorf("widgets sequence not found in column")
		}
	}

	if widgetIndex < 0 || widgetIndex >= len(widgetsNode.Content) {
		return fmt.Errorf("widget index out of bounds")
	}

	targetWidgetNode := widgetsNode.Content[widgetIndex]
	var currentWidgetMap map[string]interface{}
	if err := targetWidgetNode.Decode(&currentWidgetMap); err != nil {
		return err
	}

	widgetType, _ := currentWidgetMap["type"].(string)

	// Process and clean up Spotify credentials if present
	if widgetType == "spotify" && properties != nil {
		var clientID, clientSecret, redirectURL, accessToken, refreshToken string
		if cid, ok := properties["client_id"].(string); ok {
			clientID = strings.TrimSpace(cid)
		}
		if csec, ok := properties["client_secret"].(string); ok {
			clientSecret = strings.TrimSpace(csec)
		}
		if rurl, ok := properties["redirect_url"].(string); ok {
			redirectURL = strings.TrimSpace(rurl)
		}
		if at, ok := properties["access_token"].(string); ok {
			accessToken = strings.TrimSpace(at)
		}
		if rt, ok := properties["refresh_token"].(string); ok {
			refreshToken = strings.TrimSpace(rt)
		}

		delete(properties, "client_id")
		delete(properties, "client_secret")
		delete(properties, "redirect_url")
		delete(properties, "access_token")
		delete(properties, "refresh_token")

		if accessToken != "" && accessToken != "********" {
			_ = Store.SetSetting("spotify_access_token", accessToken)
		}
		if refreshToken != "" && refreshToken != "********" {
			_ = Store.SetSetting("spotify_refresh_token", refreshToken)
		}
		if (accessToken != "" && accessToken != "********") || (refreshToken != "" && refreshToken != "********") {
			_ = Store.SetSetting("spotify_authorized", "true")
		}

		if clientID != "" || (clientSecret != "" && clientSecret != "********") || redirectURL != "" {
			spotifyNode := findMapValue(rootMap, "spotify")
			if spotifyNode == nil || spotifyNode.Kind != yaml.MappingNode {
				spotifyNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "spotify"}
				rootMap.Content = append(rootMap.Content, keyNode, spotifyNode)
			}
			if clientID != "" {
				updateMapValue(spotifyNode, "client-id", clientID)
			}
			if clientSecret != "" && clientSecret != "********" {
				updateMapValue(spotifyNode, "client-secret", clientSecret)
			}
			if redirectURL != "" {
				updateMapValue(spotifyNode, "redirect-url", redirectURL)
			}
		}
	}

	// Process Calendar/Gmail credentials
	if (widgetType == "calendar" || widgetType == "gmail") && properties != nil {
		var clientID, clientSecret, redirectURL string
		if cid, ok := properties["google_client_id"].(string); ok {
			clientID = strings.TrimSpace(cid)
		}
		if csec, ok := properties["google_client_secret"].(string); ok {
			clientSecret = strings.TrimSpace(csec)
		}
		if rurl, ok := properties["google_redirect_url"].(string); ok {
			redirectURL = strings.TrimSpace(rurl)
		}

		delete(properties, "google_client_id")
		delete(properties, "google_client_secret")
		delete(properties, "google_redirect_url")

		if clientID != "" || (clientSecret != "" && clientSecret != "********") || redirectURL != "" {
			googleNode := findMapValue(rootMap, "google")
			if googleNode == nil || googleNode.Kind != yaml.MappingNode {
				googleNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "google"}
				rootMap.Content = append(rootMap.Content, keyNode, googleNode)
			}
			if clientID != "" {
				updateMapValue(googleNode, "client-id", clientID)
			}
			if clientSecret != "" && clientSecret != "********" {
				updateMapValue(googleNode, "client-secret", clientSecret)
			}
			if redirectURL != "" {
				updateMapValue(googleNode, "redirect-url", redirectURL)
			}
		}
	}

	// Process Hue credentials
	if widgetType == "hue" && properties != nil {
		var clientID, clientSecret, redirectURL string
		if cid, ok := properties["hue_client_id"].(string); ok {
			clientID = strings.TrimSpace(cid)
		}
		if csec, ok := properties["hue_client_secret"].(string); ok {
			clientSecret = strings.TrimSpace(csec)
		}
		if rurl, ok := properties["hue_redirect_url"].(string); ok {
			redirectURL = strings.TrimSpace(rurl)
		}

		delete(properties, "hue_client_id")
		delete(properties, "hue_client_secret")
		delete(properties, "hue_redirect_url")

		if clientID != "" || (clientSecret != "" && clientSecret != "********") || redirectURL != "" {
			hueNode := findMapValue(rootMap, "hue")
			if hueNode == nil || hueNode.Kind != yaml.MappingNode {
				hueNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "hue"}
				rootMap.Content = append(rootMap.Content, keyNode, hueNode)
			}
			if clientID != "" {
				updateMapValue(hueNode, "client-id", clientID)
			}
			if clientSecret != "" && clientSecret != "********" {
				updateMapValue(hueNode, "client-secret", clientSecret)
			}
			if redirectURL != "" {
				updateMapValue(hueNode, "redirect-url", redirectURL)
			}
		}
	}

	newWidgetMap := map[string]interface{}{
		"type": widgetType,
	}
	for k, v := range currentWidgetMap {
		if k == "type" {
			continue
		}
		newWidgetMap[k] = v
	}
	for k, v := range properties {
		if f, ok := v.(float64); ok && f == float64(int(f)) {
			switch k {
			case "update-interval", "height", "limit", "collapse-after", "thumbnail-height", "card-height", "viewport-limit", "max-days-ahead", "commits-limit", "collapse-after-rows", "pull-requests-limit", "issues-limit":
				v = int(f)
			}
		}
		if s, ok := v.(string); ok {
			if n, err := strconv.Atoi(s); err == nil {
				switch k {
				case "update-interval", "height", "limit", "collapse-after", "thumbnail-height", "card-height", "viewport-limit", "max-days-ahead", "commits-limit", "collapse-after-rows", "pull-requests-limit", "issues-limit":
					v = n
				}
			}
		}
		newWidgetMap[k] = v
	}

	newWidgetNode := &yaml.Node{}
	if err := newWidgetNode.Encode(newWidgetMap); err != nil {
		return err
	}

	widgetsNode.Content[widgetIndex] = newWidgetNode

	if err := validateASTConfig(&rootNode); err != nil {
		return err
	}

	if err := saveNodeToDisk(cm.configPath, &rootNode); err != nil {
		return err
	}

	return cm.reloadFn()
}

// DeleteWidget deletes a widget by its index.
func (cm *ConfigManager) DeleteWidget(pageSlug string, columnIndex string, widgetIndex int) error {
	cm.configFileMu.Lock()
	defer cm.configFileMu.Unlock()

	rootNode, err := cm.ReadAST()
	if err != nil {
		return err
	}

	pageNode, err := findPageNode(&rootNode, pageSlug)
	if err != nil {
		return err
	}

	var widgetsNode *yaml.Node

	if columnIndex == "head" {
		widgetsNode = findMapValue(pageNode, "head-widgets")
		if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode {
			return fmt.Errorf("head-widgets not found")
		}
	} else if strings.Contains(columnIndex, ":") {
		// Group nested widget deletion
		parts := strings.Split(columnIndex, ":")
		origColStr := parts[0]
		origIdx, _ := strconv.Atoi(parts[1])

		var baseNode *yaml.Node
		if origColStr == "head" {
			headNode := findMapValue(pageNode, "head-widgets")
			if headNode != nil && headNode.Kind == yaml.SequenceNode && origIdx >= 0 && origIdx < len(headNode.Content) {
				baseNode = headNode.Content[origIdx]
			}
		} else {
			columnsNode := findMapValue(pageNode, "columns")
			colIdx, err := strconv.Atoi(origColStr)
			if err == nil && columnsNode != nil && columnsNode.Kind == yaml.SequenceNode && colIdx >= 0 && colIdx < len(columnsNode.Content) {
				colNode := columnsNode.Content[colIdx]
				colWidgets := findMapValue(colNode, "widgets")
				if colWidgets != nil && colWidgets.Kind == yaml.SequenceNode && origIdx >= 0 && origIdx < len(colWidgets.Content) {
					baseNode = colWidgets.Content[origIdx]
				}
			}
		}

		if baseNode == nil {
			return fmt.Errorf("group widget not found")
		}

		widgetsNode = findMapValue(baseNode, "widgets")
		if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode {
			return fmt.Errorf("nested widgets sequence not found in group")
		}
	} else {
		columnsNode := findMapValue(pageNode, "columns")
		if columnsNode == nil || columnsNode.Kind != yaml.SequenceNode {
			return fmt.Errorf("columns not found")
		}

		colIdx, err := strconv.Atoi(columnIndex)
		if err != nil || colIdx < 0 || colIdx >= len(columnsNode.Content) {
			return fmt.Errorf("invalid column index")
		}

		colNode := columnsNode.Content[colIdx]
		widgetsNode = findMapValue(colNode, "widgets")
		if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode {
			return fmt.Errorf("widgets sequence not found in column")
		}
	}

	if widgetIndex < 0 || widgetIndex >= len(widgetsNode.Content) {
		return fmt.Errorf("widget index out of bounds")
	}

	widgetsNode.Content = append(widgetsNode.Content[:widgetIndex], widgetsNode.Content[widgetIndex+1:]...)

	if err := validateASTConfig(&rootNode); err != nil {
		return err
	}

	if err := saveNodeToDisk(cm.configPath, &rootNode); err != nil {
		return err
	}

	return cm.reloadFn()
}

// SaveSettings saves global application settings (branding, server, theme, spotify).
func (cm *ConfigManager) SaveSettings(branding interface{}, server interface{}, theme interface{}, spotify interface{}) error {
	cm.configFileMu.Lock()
	defer cm.configFileMu.Unlock()

	rootNode, err := cm.ReadAST()
	if err != nil {
		return err
	}

	if len(rootNode.Content) == 0 {
		return fmt.Errorf("empty YAML document")
	}
	rootMap := rootNode.Content[0]

	if err := updateTopLevelKey(rootMap, "branding", branding); err != nil {
		return fmt.Errorf("failed to update branding settings: %w", err)
	}
	if err := updateTopLevelKey(rootMap, "server", server); err != nil {
		return fmt.Errorf("failed to update server settings: %w", err)
	}
	if err := updateTopLevelKey(rootMap, "theme", theme); err != nil {
		return fmt.Errorf("failed to update theme settings: %w", err)
	}
	if err := updateTopLevelKey(rootMap, "spotify", spotify); err != nil {
		return fmt.Errorf("failed to update spotify settings: %w", err)
	}

	if err := validateASTConfig(&rootNode); err != nil {
		return err
	}

	if err := saveNodeToDisk(cm.configPath, &rootNode); err != nil {
		return err
	}

	return cm.reloadFn()
}

// Helper functions for YAML AST manipulation

func findPageNode(rootNode *yaml.Node, targetSlug string) (*yaml.Node, error) {
	if len(rootNode.Content) == 0 {
		return nil, fmt.Errorf("empty YAML document")
	}
	rootMap := rootNode.Content[0]
	pagesNode := findMapValue(rootMap, "pages")
	if pagesNode == nil || pagesNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("pages block not found or invalid")
	}

	for _, pageNode := range pagesNode.Content {
		nameNode := findMapValue(pageNode, "name")
		slugNode := findMapValue(pageNode, "slug")
		slug := ""
		if slugNode != nil {
			slug = slugNode.Value
		} else if nameNode != nil {
			slug = titleToSlug(nameNode.Value)
		}
		if slug == targetSlug {
			return pageNode, nil
		}
	}
	return nil, fmt.Errorf("page not found: %s", targetSlug)
}

func findMapValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func removeMapKey(node *yaml.Node, key string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
	}
}

func saveNodeToDisk(path string, node *yaml.Node) error {
	fixYamlScalarTypes(node)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	defer enc.Close()
	if err := enc.Encode(node); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

var yamlKnownIntKeys = map[string]bool{
	"update-interval":     true,
	"height":              true,
	"limit":               true,
	"collapse-after":      true,
	"thumbnail-height":    true,
	"card-height":         true,
	"port":                true,
	"pull-requests-limit": true,
	"issues-limit":        true,
}

func fixYamlScalarTypes(node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			if valNode.Kind == yaml.ScalarNode && valNode.Tag == "!!str" && yamlKnownIntKeys[keyNode.Value] {
				if _, err := strconv.Atoi(valNode.Value); err == nil {
					valNode.Tag = "!!int"
				}
			}
			fixYamlScalarTypes(valNode)
		}
	} else {
		for _, child := range node.Content {
			fixYamlScalarTypes(child)
		}
	}
}

func updateTopLevelKey(rootMap *yaml.Node, key string, data interface{}) error {
	if rootMap.Kind != yaml.MappingNode {
		return fmt.Errorf("root node is not a mapping node")
	}

	var newNode yaml.Node
	if err := newNode.Encode(data); err != nil {
		return err
	}

	var valNode *yaml.Node
	if newNode.Kind == yaml.DocumentNode && len(newNode.Content) > 0 {
		valNode = newNode.Content[0]
	} else {
		valNode = &newNode
	}

	for i := 0; i < len(rootMap.Content); i += 2 {
		if rootMap.Content[i].Value == key {
			rootMap.Content[i+1] = valNode
			return nil
		}
	}

	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	rootMap.Content = append(rootMap.Content, keyNode, valNode)
	return nil
}

func updateMapValue(node *yaml.Node, key string, val string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1].Value = val
			node.Content[i+1].Kind = yaml.ScalarNode
			node.Content[i+1].Tag = "!!str"
			return
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: val}
	node.Content = append(node.Content, keyNode, valNode)
}

func validateASTConfig(rootNode *yaml.Node) error {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(rootNode); err != nil {
		return fmt.Errorf("failed to encode YAML AST: %w", err)
	}

	config := NewConfig()
	if err := yaml.Unmarshal(buf.Bytes(), config); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	if err := configIsValid(config); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	return nil
}
