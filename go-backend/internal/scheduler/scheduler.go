package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/habeshahood/postiz-lite/internal/db"
	"github.com/habeshahood/postiz-lite/internal/social"
)

// Scheduler replaces the entire Temporal + orchestrator stack.
// Every minute, it checks for queued posts that are due and publishes them.
// Every hour, it refreshes expiring OAuth tokens.
type Scheduler struct {
	store         *db.Store
	ticker        *time.Ticker
	refreshTicker *time.Ticker
	done          chan struct{}
	tenantCtx     context.Context // context with tenant social keys (for token refresh)
}

func New(store *db.Store, tenantCtx context.Context) *Scheduler {
	return &Scheduler{
		store:     store,
		done:      make(chan struct{}),
		tenantCtx: tenantCtx,
	}
}

func (s *Scheduler) Start() {
	s.ticker = time.NewTicker(1 * time.Minute)
	s.refreshTicker = time.NewTicker(1 * time.Hour)
	go s.loop()
	slog.Info("scheduler started (1-minute poll, replaces Temporal)", "providers", social.Identifiers())
}

func (s *Scheduler) Stop() {
	s.ticker.Stop()
	s.refreshTicker.Stop()
	close(s.done)
	slog.Info("scheduler stopped")
}

func (s *Scheduler) loop() {
	// Run once immediately on startup
	s.processDuePosts()
	s.refreshExpiringTokens()

	for {
		select {
		case <-s.ticker.C:
			s.processDuePosts()
		case <-s.refreshTicker.C:
			s.refreshExpiringTokens()
		case <-s.done:
			return
		}
	}
}

func (s *Scheduler) processDuePosts() {
	ctx, cancel := context.WithTimeout(s.tenantCtx, 30*time.Second)
	defer cancel()

	posts, err := s.store.GetQueuedPosts(ctx)
	if err != nil {
		slog.Error("scheduler: failed to get queued posts", "error", err)
		return
	}

	if len(posts) == 0 {
		return
	}

	slog.Info("scheduler: processing due posts", "count", len(posts))

	for _, post := range posts {
		s.publishPost(ctx, post)
	}
}

func (s *Scheduler) refreshExpiringTokens() {
	ctx, cancel := context.WithTimeout(s.tenantCtx, 60*time.Second)
	defer cancel()

	integrations, err := s.store.GetExpiringIntegrations(ctx, 24*time.Hour)
	if err != nil {
		slog.Error("scheduler: failed to get expiring integrations", "error", err)
		return
	}

	for _, integration := range integrations {
		result, err := social.RefreshToken(ctx, integration)
		if err != nil {
			slog.Error("scheduler: token refresh failed",
				"integration", integration.ID,
				"provider", integration.ProviderIdentifier,
				"error", err,
			)
			continue
		}
		if result == nil {
			continue // provider doesn't support refresh
		}

		var refreshToken *string
		if result.RefreshToken != "" {
			refreshToken = &result.RefreshToken
		} else if integration.RefreshToken != nil {
			refreshToken = integration.RefreshToken // keep existing
		}

		var expiration *time.Time
		if result.ExpiresIn > 0 {
			t := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
			expiration = &t
		}

		if err := s.store.UpdateIntegrationTokens(ctx, integration.ID, result.AccessToken, refreshToken, expiration); err != nil {
			slog.Error("scheduler: failed to update tokens",
				"integration", integration.ID,
				"error", err,
			)
			continue
		}

		slog.Info("scheduler: token refreshed",
			"integration", integration.ID,
			"provider", integration.ProviderIdentifier,
			"expiresIn", result.ExpiresIn,
		)
	}
}

func (s *Scheduler) publishPost(ctx context.Context, post *db.Post) {
	integration, err := s.store.GetIntegrationByID(ctx, post.IntegrationID, post.OrganizationID)
	if err != nil {
		slog.Error("scheduler: integration not found", "postID", post.ID, "integrationID", post.IntegrationID, "error", err)
		msg := "Integration not found"
		s.store.UpdatePostState(ctx, post.ID, db.StateError, &msg)
		return
	}

	provider, err := social.Get(integration.ProviderIdentifier)
	if err != nil {
		slog.Warn("scheduler: no provider for platform",
			"postID", post.ID,
			"provider", integration.ProviderIdentifier,
		)
		msg := "Provider not supported: " + integration.ProviderIdentifier
		s.store.UpdatePostState(ctx, post.ID, db.StateError, &msg)
		return
	}

	slog.Info("scheduler: publishing post",
		"postID", post.ID,
		"provider", integration.ProviderIdentifier,
		"integration", integration.Name,
	)

	result, err := provider.Post(ctx, post, integration)
	if err != nil {
		slog.Error("scheduler: post failed",
			"postID", post.ID,
			"provider", integration.ProviderIdentifier,
			"error", err,
		)
		msg := err.Error()
		s.store.UpdatePostState(ctx, post.ID, db.StateError, &msg)
		return
	}

	if err := s.store.UpdatePostPublished(ctx, post.ID, result.PostID, result.ReleaseURL); err != nil {
		slog.Error("scheduler: failed to update post state after publish",
			"postID", post.ID,
			"error", err,
		)
		return
	}

	slog.Info("scheduler: post published",
		"postID", post.ID,
		"provider", integration.ProviderIdentifier,
		"releaseURL", result.ReleaseURL,
	)
}
