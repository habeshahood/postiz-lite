package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/habeshahood/postiz-lite/internal/db"
	"github.com/habeshahood/postiz-lite/internal/middleware"
	"github.com/habeshahood/postiz-lite/internal/tenant"
	"github.com/redis/go-redis/v9"
)

// RegisterPublic mounts routes that require NO authentication.
// These match the NestJS NoAuthIntegrationsController.
func RegisterPublic(r chi.Router, store *db.Store, rdb *redis.Client) {
	// Provider catalog — list of all available platforms (Add Channel modal)
	r.Get("/integrations", handleInternalListIntegrations(store))
	r.Get("/integrations/", handleInternalListIntegrations(store))

	// OAuth callback — uses Redis state for security, not JWT auth
	r.Post("/integrations/social-connect/{integration}", handleSocialConnectReal(store, rdb))

	// Telegram webhook + channel list
	r.Post("/webhooks/telegram", handleTelegramWebhook(store))
	r.Get("/telegram/channels", handleTelegramChannels(store))

	// Copilot / AI agent — public because CopilotKit widget doesn't send auth cookies
	r.Post("/copilot/chat", handleCopilotChat())
	r.Get("/copilot/chat", handleEmptyObject())
	r.Get("/copilot/credits", handleCopilotCredits())
}

// handleSocialConnect is now replaced by handleSocialConnectReal in social_connect.go

// RegisterInternal mounts all JWT-protected internal API routes (used by Next.js frontend).
// rdb may be nil if Redis is not configured.
func RegisterInternal(r chi.Router, store *db.Store, rdb *redis.Client) {
	// User
	r.Get("/user/self", handleSelf(store))
	r.Get("/user/organizations", handleOrganizations(store))
	r.Get("/user/personal", handlePersonal(store))
	r.Get("/user/subscription", handleSubscription())
	r.Get("/user/subscription/tiers", handleTiers())
	r.Post("/user/change-org", handleChangeOrg())
	r.Post("/user/api-key/rotate", handleRotateAPIKey(store))

	// Posts
	r.Get("/posts", handleInternalListPosts(store))
	r.Get("/posts/", handleInternalListPosts(store))
	r.Post("/posts", handleInternalCreatePost(store))
	r.Post("/posts/", handleInternalCreatePost(store))
	r.Get("/posts/find-slot", handleFindSlot())
	r.Get("/posts/find-slot/{id}", handleFindSlot())
	r.Get("/posts/list", handleInternalListPosts(store))
	r.Get("/posts/old", handleOldPosts(store))
	r.Get("/posts/group/{group}", handlePostGroup(store))
	r.Get("/posts/tags", handleGetTags())
	r.Post("/posts/should-shortlink", handleShouldShortlink())
	r.Delete("/posts/{group}", handleDeletePostGroup(store))
	r.Put("/posts/{id}/date", handleChangeDate(store))
	r.Get("/posts/{id}/statistics", handleEmptyObject())
	r.Get("/posts/{id}/comments", handleListComments(store))
	r.Post("/posts/{id}/comments", handleCreateComment(store))

	// Integrations (GET /integrations is public — see RegisterPublic)
	r.Get("/integrations/list", handleIntegrationsList(store))
	r.Get("/integrations/social/{integration}", handleSocialOAuthURLReal(store, rdb))
	r.Get("/integrations/{id}", handleGetIntegration(store))
	r.Get("/integrations/{id}/internal-plugs", handleEmptyArray())
	r.Post("/integrations/disable", handleIntegrationToggle(store, true))
	r.Post("/integrations/enable", handleIntegrationToggle(store, false))
	r.Delete("/integrations", handleDeleteIntegrationInternal(store))
	r.Delete("/integrations/", handleDeleteIntegrationInternal(store))

	// Notifications
	r.Get("/notifications", handleNotificationsList())
	r.Get("/notifications/", handleNotificationsList())
	r.Get("/notifications/list", handleNotificationsList())

	// Analytics (stub)
	r.Get("/analytics/{integration}", handleAnalytics())
	r.Get("/analytics/post/{postId}", handlePostAnalytics())

	// Settings
	r.Get("/settings/team", handleSettingsTeam(store))
	r.Get("/settings/shortlink", handleShortlink())
	r.Get("/settings/signatures", handleSignatures())
	r.Get("/settings/webhooks", handleListWebhooks(store))
	r.Post("/settings/webhooks", handleCreateWebhook(store))
	r.Put("/settings/webhooks", handleUpdateWebhook(store))
	r.Delete("/settings/webhooks/{id}", handleDeleteWebhook(store))
	r.Get("/settings/auto-post", handleListAutoPosts(store))
	r.Post("/settings/auto-post", handleCreateAutoPost(store))
	r.Put("/settings/auto-post/{id}", handleUpdateAutoPost(store))
	r.Delete("/settings/auto-post/{id}", handleDeleteAutoPost(store))
	r.Post("/settings/auto-post/{id}/active", handleSetAutoPostActive(store))

	// Sets (content sets — used by calendar for grouping)
	r.Get("/sets", handleGetSets())
	r.Get("/sets/", handleGetSets())
	r.Post("/sets", handleGetSets())
	r.Post("/sets/", handleGetSets())

	// Signatures (post signatures/footers)
	r.Get("/signatures", handleSignatures())
	r.Get("/signatures/", handleSignatures())
	r.Get("/signatures/default", handleDefaultSignature())

	// Copilot / AI agent (chat/credits are public — see RegisterPublic)
	r.Get("/copilot/list", handleEmptyArray())
	r.Get("/copilot/agent", handleEmptyArray())
	r.Post("/copilot/agent", handleEmptyArray())

	// Third-party integrations
	r.Get("/third-party", handleEmptyArray())
	r.Get("/third-party/", handleEmptyArray())
	r.Get("/third-party/list", handleEmptyArray())

	// Media library
	r.Get("/media", handleListMedia(store))
	r.Get("/media/", handleEmptyArray())
	r.Get("/media/video-options", handleEmptyArray())
	r.Post("/media/upload-simple", handleUpload(store))
	r.Post("/media/upload-server", handleUpload(store))
	r.Post("/media/save-media", handleSaveMedia(store))
	r.Delete("/media/{id}", handleDeleteMedia(store))

	// Plugs (automation plugins)
	r.Get("/plugs", handleEmptyArray())
	r.Get("/plugs/", handleEmptyArray())
	r.Get("/integrations/plug/list", handleEmptyArray())

	// Autopost
	r.Get("/autopost", handleEmptyArray())
	r.Get("/autopost/", handleEmptyArray())

	// Customers
	r.Get("/customers", handleEmptyArray())
	r.Get("/customers/", handleEmptyArray())

	// Webhooks
	r.Get("/webhooks", handleEmptyArray())
	r.Get("/webhooks/", handleEmptyArray())

	// Approved apps (OAuth)
	r.Get("/approved-apps", handleEmptyArray())
	r.Get("/user/approved-apps", handleEmptyArray())
	r.Put("/integrations", handleUpdateIntegration(store))
	r.Put("/integrations/", handleUpdateIntegration(store))

	// User extras
	r.Get("/user/email-notifications", handleEmptyObject())
	r.Post("/user/email-notifications", handleEmptyObject())
}

// ---- User handlers ----

func handleSelf(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.GetUser(r)
		org := middleware.GetOrg(r)
		if user == nil {
			http.Error(w, `{"msg":"Not authenticated"}`, http.StatusUnauthorized)
			return
		}

		role := db.RoleUser
		publicAPI := ""
		if org != nil {
			if r, err := store.GetUserRoleInOrg(r.Context(), user.ID, org.ID); err == nil {
				role = r
			}
			if role == db.RoleSuperAdmin || role == db.RoleAdmin {
				if org.APIKey != nil {
					publicAPI = *org.APIKey
				}
			}
		}

		orgID := ""
		if org != nil {
			orgID = org.ID
		}

		// Match the exact shape the frontend expects from /user/self
		writeJSON(w, http.StatusOK, map[string]any{
			"id":            user.ID,
			"email":         user.Email,
			"name":          user.Name,
			"lastName":      user.LastName,
			"providerName":  user.ProviderName,
			"timezone":      user.Timezone,
			"activated":     user.Activated,
			"orgId":         orgID,
			"totalChannels": 10000,     // No Stripe = unlimited
			"tier":          "ULTIMATE", // No billing = top tier
			"role":          role,
			"isLifetime":    false,
			"admin":         false,
			"impersonate":   false,
			"isTrailing":    false,
			"allowTrial":    true,
			"streakSince":   nil,
			"publicApi":     publicAPI,
		})
	}
}

func handleOrganizations(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.GetUser(r)
		if user == nil {
			http.Error(w, `{"msg":"Not authenticated"}`, http.StatusUnauthorized)
			return
		}

		orgs, err := store.GetUserOrganizations(r.Context(), user.ID)
		if err != nil {
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}
		if orgs == nil {
			orgs = []db.UserOrg{}
		}

		// Shape expected by frontend: array of {id, name, users: [{role, disabled}]}
		result := make([]map[string]any, 0, len(orgs))
		for _, o := range orgs {
			result = append(result, map[string]any{
				"id":   o.OrgID,
				"name": o.OrgName,
				"users": []map[string]any{
					{"role": o.Role, "disabled": o.Disabled},
				},
			})
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handlePersonal(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.GetUser(r)
		if user == nil {
			http.Error(w, `{"msg":"Not authenticated"}`, http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
		})
	}
}

func handleSubscription() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// No Stripe = ULTIMATE tier, unlimited everything
		writeJSON(w, http.StatusOK, map[string]any{
			"subscriptionTier": "ULTIMATE",
			"totalChannels":    10000,
			"isLifetime":       false,
		})
	}
}

func handleTiers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	}
}

func handleChangeOrg() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// In a real impl, this would switch the org cookie.
		// For now, acknowledge the request.
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func handleRotateAPIKey(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Stub — would regenerate the org API key
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ---- Post handlers ----

func handleInternalListPosts(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}

		// Calendar view sends ?date=YYYY-MM-DD for month view
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")

		var posts []*db.Post
		var err error
		if from != "" && to != "" {
			posts, err = store.GetPostsByDateRange(r.Context(), org.ID, from, to)
		} else {
			posts, err = store.ListPosts(r.Context(), org.ID)
		}

		if err != nil {
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}
		if posts == nil {
			posts = []*db.Post{}
		}

		// Group posts by their group field (calendar view expects grouped structure)
		type postGroup struct {
			Group     string     `json:"group"`
			Posts     []*db.Post `json:"posts"`
			State     string     `json:"state"`
			Date      string     `json:"date"`
			IntegID   string     `json:"integrationId"`
		}

		groups := map[string]*postGroup{}
		for _, p := range posts {
			g, ok := groups[p.Group]
			if !ok {
				g = &postGroup{
					Group:   p.Group,
					Posts:   []*db.Post{},
					State:   string(p.State),
					Date:    p.PublishDate.Format("2006-01-02T15:04:05.000Z"),
					IntegID: p.IntegrationID,
				}
				groups[p.Group] = g
			}
			g.Posts = append(g.Posts, p)
		}

		result := make([]any, 0, len(groups))
		for _, g := range groups {
			result = append(result, g)
		}

		writeJSON(w, http.StatusOK, result)
	}
}

// ---- NestJS CreatePostDto types ----

type createPostRequest struct {
	Type      string          `json:"type"`      // "draft", "schedule", "now", "update"
	Date      string          `json:"date"`      // ISO 8601
	Tags      json.RawMessage `json:"tags"`      // ignored for now
	ShortLink bool            `json:"shortLink"` // ignored
	Posts     []postEntry     `json:"posts"`
}

type postEntry struct {
	Integration struct {
		ID string `json:"id"`
	} `json:"integration"`
	Value    []postValue     `json:"value"`
	Settings json.RawMessage `json:"settings"`
	Group    string          `json:"group"` // empty for new, set for update
}

type postValue struct {
	Content string          `json:"content"`
	ID      string          `json:"id"`    // empty for create, set for update
	Delay   int             `json:"delay"`
	Image   json.RawMessage `json:"image"` // JSON array of media objects
}

func handleInternalCreatePost(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}

		var req createPostRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"msg":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}

		// Determine publish date
		var publishDate time.Time
		if req.Type == "now" {
			publishDate = time.Now().UTC()
		} else if req.Date != "" {
			var err error
			publishDate, err = time.Parse(time.RFC3339, req.Date)
			if err != nil {
				// Try millisecond variant
				publishDate, err = time.Parse("2006-01-02T15:04:05.000Z", req.Date)
				if err != nil {
					publishDate = time.Now().UTC()
				}
			}
		} else {
			publishDate = time.Now().UTC()
		}

		type createdEntry struct {
			PostID      string `json:"postId"`
			Integration string `json:"integration"`
		}
		result := make([]createdEntry, 0)

		for _, entry := range req.Posts {
			integrationID := entry.Integration.ID
			if integrationID == "" {
				http.Error(w, `{"msg":"integration.id is required"}`, http.StatusBadRequest)
				return
			}

			// Stringify settings once per entry
			var settingsStr *string
			if len(entry.Settings) > 0 && string(entry.Settings) != "null" {
				s := string(entry.Settings)
				settingsStr = &s
			}

			// Shared group UUID for all value[] entries in this post entry
			group := entry.Group
			if group == "" {
				group = uuid.New().String()
			}

			if req.Type == "update" {
				// Update existing posts matched by value[].id
				for _, v := range entry.Value {
					if v.ID == "" {
						continue
					}
					var imageStr *string
					if len(v.Image) > 0 && string(v.Image) != "null" {
						s := string(v.Image)
						imageStr = &s
					}
					updated, err := store.UpdatePostContent(r.Context(), v.ID, org.ID, v.Content, imageStr, settingsStr)
					if err != nil {
						http.Error(w, `{"msg":"Failed to update post"}`, http.StatusInternalServerError)
						return
					}
					result = append(result, createdEntry{
						PostID:      updated.ID,
						Integration: integrationID,
					})
				}
				continue
			}

			// Create path: chain value[] entries as a thread
			var prevPostID *string
			for _, v := range entry.Value {
				var imageStr *string
				if len(v.Image) > 0 && string(v.Image) != "null" {
					s := string(v.Image)
					imageStr = &s
				}

				created, err := store.CreatePost(
					r.Context(),
					org.ID,
					integrationID,
					v.Content,
					group,
					publishDate,
					nil,         // title
					settingsStr, // settings
					imageStr,    // image
					prevPostID,  // parentPostId (nil for first)
				)
				if err != nil {
					http.Error(w, `{"msg":"Failed to create post"}`, http.StatusInternalServerError)
					return
				}

				// If draft, update state (CreatePost always inserts as QUEUE)
				if req.Type == "draft" {
					if err := store.UpdatePostState(r.Context(), created.ID, db.StateDraft, nil); err != nil {
						http.Error(w, `{"msg":"Failed to set draft state"}`, http.StatusInternalServerError)
						return
					}
					created.State = db.StateDraft
				}

				result = append(result, createdEntry{
					PostID:      created.ID,
					Integration: integrationID,
				})

				// Thread: next value[] entry's parent = this post
				id := created.ID
				prevPostID = &id
			}
		}

		writeJSON(w, http.StatusCreated, result)
	}
}

func handleFindSlot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Next hour, rounded up
		next := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
		writeJSON(w, http.StatusOK, map[string]any{
			"date": next.Format("2006-01-02T15:04:05.000Z"),
		})
	}
}

func handleListMedia(store *db.Store) http.HandlerFunc {
	const perPage = 18
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		page := 1
		if p := r.URL.Query().Get("page"); p != "" {
			fmt.Sscanf(p, "%d", &page)
		}
		if page < 1 {
			page = 1
		}

		media, err := store.ListMedia(r.Context(), org.ID)
		if err != nil {
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}
		if media == nil {
			media = []*db.Media{}
		}

		total := len(media)
		pages := (total + perPage - 1) / perPage
		start := (page - 1) * perPage
		end := start + perPage
		if start > total {
			start = total
		}
		if end > total {
			end = total
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"pages":   pages,
			"results": media[start:end],
		})
	}
}

// handleSaveMedia registers an uploaded file in the Media table.
// Called by the frontend after Uppy uploads to /media/upload-server.
func handleSaveMedia(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}

		var body struct {
			Name         string `json:"name"`
			OriginalName string `json:"originalName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			writeJSON(w, http.StatusOK, false)
			return
		}

		uploadDir := tenant.GetUploadDir(r.Context())
		if uploadDir == "" {
			uploadDir = "/uploads"
		}

		// The file was already saved by handleUpload at uploadDir/name
		// Store URL-relative path, not filesystem path
		filePath := "/uploads/" + body.Name

		var origName *string
		if body.OriginalName != "" {
			origName = &body.OriginalName
		}

		media, err := store.SaveMedia(r.Context(), org.ID, body.Name, filePath, "image", origName, 0)
		if err != nil {
			http.Error(w, `{"msg":"Failed to save media"}`, http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":           media.ID,
			"name":         media.Name,
			"originalName": media.OriginalName,
			"path":         media.Path,
			"thumbnail":    media.Thumbnail,
		})
	}
}

func handleOldPosts(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		posts, err := store.ListPosts(r.Context(), org.ID)
		if err != nil {
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}
		if posts == nil {
			posts = []*db.Post{}
		}
		writeJSON(w, http.StatusOK, posts)
	}
}

func handlePostGroup(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		group := chi.URLParam(r, "group")
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		posts, err := store.ListPosts(r.Context(), org.ID)
		if err != nil {
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}
		// Filter by group
		var grouped []*db.Post
		for _, p := range posts {
			if p.Group == group {
				grouped = append(grouped, p)
			}
		}
		if grouped == nil {
			grouped = []*db.Post{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"group": group,
			"posts": grouped,
		})
	}
}

func handleGetTags() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	}
}

func handleDeletePostGroup(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		group := chi.URLParam(r, "group")
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		// Soft-delete all posts in the group
		const q = `UPDATE "Post" SET "deletedAt" = NOW(), "updatedAt" = NOW()
			WHERE "group" = $1 AND "organizationId" = $2 AND "deletedAt" IS NULL`
		store.Exec(r.Context(), q, group, org.ID)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ---- Integration handlers ----

// handleInternalListIntegrations returns the provider catalog — the list of all
// available social platforms (X, TikTok, YouTube, etc.) that users can connect.
// This is called by the "Add Channel" modal on GET /integrations (no /list suffix).
func handleInternalListIntegrations(store *db.Store) http.HandlerFunc {
	// Static catalog of available providers — matches socialIntegrationList in the NestJS code.
	type customField struct {
		Key          string `json:"key"`
		Label        string `json:"label"`
		DefaultValue string `json:"defaultValue,omitempty"`
		Validation   string `json:"validation"`
		Type         string `json:"type"`
	}
	type providerInfo struct {
		Name              string        `json:"name"`
		Identifier        string        `json:"identifier"`
		ToolTip           string        `json:"toolTip,omitempty"`
		Editor            string        `json:"editor"`
		IsExternal        bool          `json:"isExternal"`
		IsWeb3            bool          `json:"isWeb3"`
		IsChromeExtension bool          `json:"isChromeExtension"`
		CustomFields      []customField `json:"customFields,omitempty"`
	}

	catalog := map[string]any{
		"social": []providerInfo{
			{Name: "Instagram", Identifier: "instagram", Editor: "normal"},
			{Name: "X", Identifier: "x", Editor: "normal"},
			{Name: "Facebook Page", Identifier: "facebook", Editor: "normal"},
			{Name: "YouTube", Identifier: "youtube", Editor: "normal"},
			{Name: "Telegram", Identifier: "telegram", Editor: "normal"},
			{Name: "TikTok", Identifier: "tiktok", Editor: "normal"},
		},
		"article": []any{},
	}
	// Original full catalog kept for reference — only 6 platforms enabled for 1hood
	_ = map[string]any{
		"social_full_unused": []providerInfo{
			{Name: "X", Identifier: "x", Editor: "normal", ToolTip: "You will be logged in into your current account"},
			{Name: "LinkedIn", Identifier: "linkedin", Editor: "normal"},
			{Name: "LinkedIn Page", Identifier: "linkedin-page", Editor: "normal"},
			{Name: "Reddit", Identifier: "reddit", Editor: "markdown"},
			{Name: "Instagram", Identifier: "instagram", Editor: "normal"},
			{Name: "Instagram Standalone", Identifier: "instagram-standalone", Editor: "normal"},
			{Name: "Facebook Page", Identifier: "facebook", Editor: "normal"},
			{Name: "Threads", Identifier: "threads", Editor: "normal"},
			{Name: "YouTube", Identifier: "youtube", Editor: "normal"},
			{Name: "Google My Business", Identifier: "gmb", Editor: "normal"},
			{Name: "TikTok", Identifier: "tiktok", Editor: "normal"},
			{Name: "Pinterest", Identifier: "pinterest", Editor: "normal"},
			{Name: "Dribbble", Identifier: "dribbble", Editor: "normal"},
			{Name: "Discord", Identifier: "discord", Editor: "normal"},
			{Name: "Slack", Identifier: "slack", Editor: "normal"},
			{Name: "Kick", Identifier: "kick", Editor: "normal", IsChromeExtension: true},
			{Name: "Twitch", Identifier: "twitch", Editor: "normal", IsChromeExtension: true},
			{Name: "Mastodon", Identifier: "mastodon", Editor: "normal", IsExternal: true},
			{Name: "Bluesky", Identifier: "bluesky", Editor: "normal", CustomFields: []customField{
				{Key: "service", Label: "Service", DefaultValue: "https://bsky.social", Validation: `/^(https?:\\/\\/)?[a-zA-Z0-9.-]+(:[0-9]+)?$/`, Type: "text"},
				{Key: "identifier", Label: "Identifier", Validation: `/^.+$/`, Type: "text"},
				{Key: "password", Label: "Password", Validation: `/^.{3,}$/`, Type: "password"},
			}},
			{Name: "Lemmy", Identifier: "lemmy", Editor: "markdown", IsExternal: true},
			{Name: "Farcaster", Identifier: "farcaster", Editor: "normal", IsWeb3: true},
			{Name: "Telegram", Identifier: "telegram", Editor: "normal"},
			{Name: "Nostr", Identifier: "nostr", Editor: "normal", IsWeb3: true},
			{Name: "VK", Identifier: "vk", Editor: "normal"},
			{Name: "Medium", Identifier: "medium", Editor: "html"},
			{Name: "Dev.to", Identifier: "dev.to", Editor: "markdown"},
			{Name: "Hashnode", Identifier: "hashnode", Editor: "markdown"},
			{Name: "WordPress", Identifier: "wordpress", Editor: "html"},
			{Name: "Listmonk", Identifier: "listmonk", Editor: "html"},
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, catalog)
	}
}

func handleIntegrationsList(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}

		integrations, err := store.ListIntegrations(r.Context(), org.ID)
		if err != nil {
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}

		// Build response matching the exact shape the frontend expects
		result := make([]map[string]any, 0, len(integrations))
		for _, i := range integrations {
			picture := "/no-picture.jpg"
			if i.Picture != nil && *i.Picture != "" {
				picture = *i.Picture
			}

			additionalSettings := "[]"
			if i.AdditionalSettings != nil {
				additionalSettings = *i.AdditionalSettings
			}

			// Map providerIdentifier to editor type
			editor := "normal"

			result = append(result, map[string]any{
				"name":                 i.Name,
				"id":                   i.ID,
				"internalId":           i.InternalID,
				"disabled":             i.Disabled,
				"editor":               editor,
				"picture":              picture,
				"identifier":           i.ProviderIdentifier,
				"inBetweenSteps":       i.InBetweenSteps,
				"refreshNeeded":        i.RefreshNeeded,
				"isCustomFields":       false,
				"display":              i.Profile,
				"type":                 i.Type,
				"time":                 json.RawMessage(`[{"time":120},{"time":400},{"time":700}]`),
				"changeProfilePicture": false,
				"changeNickName":       false,
				"customer":             nil,
				"additionalSettings":   additionalSettings,
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"integrations": result,
		})
	}
}

// handleSocialOAuthURL is now replaced by handleSocialOAuthURLReal in oauth.go

func handleGetIntegration(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		id := chi.URLParam(r, "id")
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		integration, err := store.GetIntegrationByID(r.Context(), id, org.ID)
		if err != nil {
			http.Error(w, `{"msg":"Not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, integration)
	}
}

func handleIntegrationToggle(store *db.Store, disable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Stub
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func handleDeleteIntegrationInternal(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, `{"msg":"Invalid request"}`, http.StatusBadRequest)
			return
		}
		if err := store.DeleteIntegration(r.Context(), body.ID, org.ID); err != nil {
			http.Error(w, `{"msg":"Failed to delete"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func handleUpdateIntegration(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"msg":"Invalid request"}`, http.StatusBadRequest)
			return
		}
		id, _ := body["id"].(string)
		if id == "" {
			http.Error(w, `{"msg":"Missing id"}`, http.StatusBadRequest)
			return
		}
		// Update name if provided
		if name, ok := body["name"].(string); ok && name != "" {
			store.Exec(r.Context(), `UPDATE "Integration" SET name = $1, "updatedAt" = NOW() WHERE id = $2 AND "organizationId" = $3`, name, id, org.ID)
		}
		// Update additionalSettings if provided
		if settings, ok := body["additionalSettings"].(string); ok {
			store.Exec(r.Context(), `UPDATE "Integration" SET "additionalSettings" = $1, "updatedAt" = NOW() WHERE id = $2 AND "organizationId" = $3`, settings, id, org.ID)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ---- Notification handlers ----

func handleNotificationsList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	}
}

// ---- Analytics handlers ----

func handleAnalytics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	}
}

func handlePostAnalytics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	}
}

// ---- Settings handlers ----

func handleSettingsTeam(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	}
}

func handleShortlink() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"shortlink": ""})
	}
}

func handleSignatures() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	}
}

func handleDefaultSignature() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{})
	}
}

func handleGetSets() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	}
}

// handleCopilotChat proxies CopilotKit requests to local Ollama.
// Ollama's OpenAI-compatible endpoint handles the chat completions.
func handleCopilotChat() http.HandlerFunc {
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://127.0.0.1:11434"
	}
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen3.5:9b"
	}

	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"msg":"Bad request"}`, http.StatusBadRequest)
			return
		}

		// Parse and inject system prompt + force our model
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, `{"msg":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}
		req["model"] = model
		req["stream"] = true

		// Prepend social media system prompt
		if msgs, ok := req["messages"].([]any); ok {
			hasSystem := false
			for _, m := range msgs {
				if msg, ok := m.(map[string]any); ok && msg["role"] == "system" {
					hasSystem = true
					break
				}
			}
			if !hasSystem {
				prompt := tenant.GetAIPrompt(r.Context())
				if prompt == "" {
					prompt = "You are a social media content assistant. Help users write engaging posts for X (Twitter), TikTok, YouTube, Facebook, and Instagram. Be creative, concise, and match the platform's style. Use hashtags where appropriate. Keep tweets under 280 characters."
				}
				sys := map[string]any{
					"role":    "system",
					"content": prompt,
				}
				req["messages"] = append([]any{sys}, msgs...)
			}
		}

		modified, _ := json.Marshal(req)

		proxyReq, _ := http.NewRequestWithContext(r.Context(), "POST",
			ollamaURL+"/v1/chat/completions", bytes.NewReader(modified))
		proxyReq.Header.Set("Content-Type", r.Header.Get("Content-Type"))

		resp, err := http.DefaultClient.Do(proxyReq)
		if err != nil {
			slog.Error("copilot: runtime request failed", "error", err)
			http.Error(w, `{"msg":"AI service unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		defer resp.Body.Close()

		// Stream response back
		for k, v := range resp.Header {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
		w.WriteHeader(resp.StatusCode)

		flusher, _ := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				if flusher != nil {
					flusher.Flush()
				}
			}
			if err != nil {
				break
			}
		}
	}
}

func handleCopilotCredits() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"credits": 999999})
	}
}

func handleChangeDate(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")

		var body struct {
			Date   string `json:"date"`
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"msg":"Invalid request"}`, http.StatusBadRequest)
			return
		}

		publishDate, err := time.Parse(time.RFC3339, body.Date)
		if err != nil {
			http.Error(w, `{"msg":"Invalid date format"}`, http.StatusBadRequest)
			return
		}

		if body.Action == "schedule" || body.Action == "" {
			store.Exec(r.Context(),
				`UPDATE "Post" SET "publishDate" = $1, state = 'QUEUE', "updatedAt" = NOW() WHERE id = $2 AND "organizationId" = $3`,
				publishDate, id, org.ID)
		} else {
			store.Exec(r.Context(),
				`UPDATE "Post" SET "publishDate" = $1, "updatedAt" = NOW() WHERE id = $2 AND "organizationId" = $3`,
				publishDate, id, org.ID)
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleDeleteMedia(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		store.Exec(r.Context(),
			`UPDATE "Media" SET "deletedAt" = NOW() WHERE id = $1 AND "organizationId" = $2`,
			id, org.ID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// handleEmptyArray returns [] — used as a stub for list endpoints.
// handleShouldShortlink stubs the short-link check.
// The NestJS version checks if messages should be auto-shortened. We always say no.
func handleShouldShortlink() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ask": false})
	}
}

func handleEmptyArray() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	}
}

// handleEmptyObject returns {} — used as a stub for detail endpoints.
func handleEmptyObject() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{})
	}
}

// ---- Webhook handlers ----

func handleListWebhooks(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		webhooks, err := store.ListWebhooks(r.Context(), org.ID)
		if err != nil {
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}
		if webhooks == nil {
			webhooks = []*db.Webhook{}
		}
		writeJSON(w, http.StatusOK, webhooks)
	}
}

func handleCreateWebhook(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Name == "" || body.URL == "" {
			http.Error(w, `{"msg":"name and url required"}`, http.StatusBadRequest)
			return
		}
		wh, err := store.CreateWebhook(r.Context(), org.ID, body.Name, body.URL)
		if err != nil {
			http.Error(w, `{"msg":"Failed to create webhook"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, wh)
	}
}

func handleUpdateWebhook(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID == "" || body.Name == "" || body.URL == "" {
			http.Error(w, `{"msg":"id, name and url required"}`, http.StatusBadRequest)
			return
		}
		if err := store.UpdateWebhook(r.Context(), org.ID, body.ID, body.Name, body.URL); err != nil {
			http.Error(w, `{"msg":"Failed to update webhook"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleDeleteWebhook(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		if err := store.DeleteWebhook(r.Context(), org.ID, id); err != nil {
			http.Error(w, `{"msg":"Failed to delete webhook"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// ---- Auto-post handlers ----

func handleListAutoPosts(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		autoposts, err := store.ListAutoPosts(r.Context(), org.ID)
		if err != nil {
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}
		if autoposts == nil {
			autoposts = []*db.AutoPost{}
		}
		writeJSON(w, http.StatusOK, autoposts)
	}
}

func handleCreateAutoPost(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"msg":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}
		ap, err := store.CreateAutoPost(r.Context(), org.ID, body)
		if err != nil {
			http.Error(w, `{"msg":"Failed to create auto-post"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, ap)
	}
}

func handleUpdateAutoPost(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"msg":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}
		ap, err := store.UpdateAutoPost(r.Context(), org.ID, id, body)
		if err != nil {
			http.Error(w, `{"msg":"Failed to update auto-post"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, ap)
	}
}

func handleDeleteAutoPost(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		if err := store.DeleteAutoPost(r.Context(), org.ID, id); err != nil {
			http.Error(w, `{"msg":"Failed to delete auto-post"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleSetAutoPostActive(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		var body struct {
			Active bool `json:"active"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if err := store.SetAutoPostActive(r.Context(), org.ID, id, body.Active); err != nil {
			http.Error(w, `{"msg":"Failed to update auto-post active state"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// ---- Comment handlers ----

func handleListComments(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		postID := chi.URLParam(r, "id")
		comments, err := store.ListComments(r.Context(), postID, org.ID)
		if err != nil {
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}
		if comments == nil {
			comments = []*db.Comment{}
		}
		writeJSON(w, http.StatusOK, comments)
	}
}

func handleCreateComment(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		user := middleware.GetUser(r)
		if user == nil {
			http.Error(w, `{"msg":"Not authenticated"}`, http.StatusUnauthorized)
			return
		}
		postID := chi.URLParam(r, "id")
		var body struct {
			Content string `json:"content"`
			Comment string `json:"comment"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		text := body.Content
		if text == "" {
			text = body.Comment
		}
		if text == "" {
			http.Error(w, `{"msg":"content required"}`, http.StatusBadRequest)
			return
		}
		c, err := store.CreateComment(r.Context(), org.ID, postID, user.ID, text)
		if err != nil {
			http.Error(w, `{"msg":"Failed to create comment"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
}
