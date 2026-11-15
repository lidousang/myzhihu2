package service

import (
	"context"
	"strings"
	"sync"

	"myzhihu2/repository/cache"
	"myzhihu2/repository/dao"
	"myzhihu2/repository/model"
	"myzhihu2/serializer"
	"myzhihu2/util/global"

	"gorm.io/gorm"
)

var (
	// ContentSrvIns is the shared, stateless content service instance.
	ContentSrvIns  *ContentSrv
	contentSrvOnce sync.Once
)

// ContentSrv contains content creation, reading, editing, and deletion rules.
type ContentSrv struct{}

// GetContentSrv returns the singleton content service.
func GetContentSrv() *ContentSrv {
	contentSrvOnce.Do(func() { ContentSrvIns = &ContentSrv{} })
	return ContentSrvIns
}

// CreateContent creates a root question or article. The author ID is provided
// by the authenticated API context, and the created event is published only
// after the MySQL insert succeeds.
func (s *ContentSrv) CreateContent(ctx context.Context, authorID uint, req serializer.CreateContentRequest) (*serializer.ContentResponse, error) {
	if authorID == 0 || (req.Kind != string(model.ContentKindQuestion) && req.Kind != string(model.ContentKindArticle)) {
		return nil, ErrInvalidKind
	}
	title, body := strings.TrimSpace(req.Title), strings.TrimSpace(req.Body)
	if title == "" || body == "" || len(title) > 255 || !model.ValidTopic(req.Topic) {
		return nil, ErrInvalidInput
	}
	db, err := dao.NewContentDao(ctx)
	if err != nil {
		return nil, err
	}
	content := &model.Content{AuthorID: authorID, Kind: model.ContentKind(req.Kind), Topic: req.Topic, Title: title, Body: body}
	if err := global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(content).Error; err != nil {
			return err
		}
		return createOutboxTx(ctx, tx, "content.created", "feed.content", map[string]any{
			"content_id": content.ID, "kind": content.Kind, "author_id": content.AuthorID,
		})
	}); err != nil {
		return nil, err
	}
	content, err = db.FindByID(content.ID)
	if err != nil {
		return nil, err
	}
	response := buildContentResponse(content)
	return &response, nil
}

// CreateAnswer validates that the parent exists and is a question before
// inserting an answer row with parent_id set to that question.
func (s *ContentSrv) CreateAnswer(ctx context.Context, authorID, questionID uint, req serializer.CreateAnswerRequest) (*serializer.ContentResponse, error) {
	if authorID == 0 || questionID == 0 || strings.TrimSpace(req.Body) == "" {
		return nil, ErrInvalidInput
	}
	db, err := dao.NewContentDao(ctx)
	if err != nil {
		return nil, err
	}
	question, err := db.FindByID(questionID)
	if dao.IsNotFound(err) {
		return nil, ErrContentNotFound
	}
	if err != nil {
		return nil, err
	}
	if question.Kind != model.ContentKindQuestion {
		return nil, ErrInvalidParent
	}
	// Answers inherit the question's topic so they join the same interest stream.
	content := &model.Content{AuthorID: authorID, Kind: model.ContentKindAnswer, ParentID: &questionID, Topic: question.Topic, Body: strings.TrimSpace(req.Body)}
	if err := global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(content).Error; err != nil {
			return err
		}
		return createOutboxTx(ctx, tx, "content.created", "feed.content", map[string]any{
			"content_id": content.ID, "kind": content.Kind, "author_id": content.AuthorID,
		})
	}); err != nil {
		return nil, err
	}
	content, err = db.FindByID(content.ID)
	if err != nil {
		return nil, err
	}
	response := buildContentResponse(content)
	return &response, nil
}

// GetContent returns a content item by ID and converts its GORM model to a
// public DTO, avoiding direct model serialization.
func (s *ContentSrv) GetContent(ctx context.Context, id uint) (*serializer.ContentResponse, error) {
	db, err := dao.NewContentDao(ctx)
	if err != nil {
		return nil, err
	}
	content, err := db.FindByID(id)
	if dao.IsNotFound(err) {
		return nil, ErrContentNotFound
	}
	if err != nil {
		return nil, err
	}
	response := buildContentResponse(content)
	return &response, nil
}

// UpdateContent allows only the author to edit title/body. The service builds a
// whitelist map so callers cannot modify author_id, kind, or parent_id.
func (s *ContentSrv) UpdateContent(ctx context.Context, actorID, contentID uint, req serializer.UpdateContentRequest) (*serializer.ContentResponse, error) {
	db, err := dao.NewContentDao(ctx)
	if err != nil {
		return nil, err
	}
	content, err := db.FindByID(contentID)
	if dao.IsNotFound(err) {
		return nil, ErrContentNotFound
	}
	if err != nil {
		return nil, err
	}
	if content.AuthorID != actorID {
		return nil, ErrForbidden
	}
	fields := make(map[string]any)
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" || len(title) > 255 || (content.Kind != model.ContentKindQuestion && content.Kind != model.ContentKindArticle) {
			return nil, ErrInvalidInput
		}
		fields["title"] = title
	}
	if req.Body != nil {
		body := strings.TrimSpace(*req.Body)
		if body == "" {
			return nil, ErrInvalidInput
		}
		fields["body"] = body
	}
	if len(fields) > 0 {
		if err := global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.Content{}).Where("id = ?", contentID).Updates(fields).Error; err != nil {
				return err
			}
			return createOutboxTx(ctx, tx, "content.updated", "feed.content", map[string]any{
				"content_id": content.ID, "kind": content.Kind, "author_id": content.AuthorID,
			})
		}); err != nil {
			return nil, err
		}
	}
	updated, err := db.FindByID(contentID)
	if err != nil {
		return nil, err
	}
	response := buildContentResponse(updated)
	return &response, nil
}

// DeleteContent soft-deletes an item after checking author ownership. The
// deletion event lets a future Feed consumer clean or skip stale references.
func (s *ContentSrv) DeleteContent(ctx context.Context, actorID, contentID uint) error {
	db, err := dao.NewContentDao(ctx)
	if err != nil {
		return err
	}
	content, err := db.FindByID(contentID)
	if dao.IsNotFound(err) {
		return ErrContentNotFound
	}
	if err != nil {
		return err
	}
	if content.AuthorID != actorID {
		return ErrForbidden
	}
	if err := global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.Content{}, contentID).Error; err != nil {
			return err
		}
		return createOutboxTx(ctx, tx, "content.deleted", "feed.content", map[string]any{
			"content_id": content.ID, "kind": content.Kind, "author_id": content.AuthorID,
		})
	}); err != nil {
		return err
	}
	return nil
}

// ListQuestionAnswers verifies the parent kind, then reads a paginated answer list.
func (s *ContentSrv) ListQuestionAnswers(ctx context.Context, questionID uint, page, limit int) (*serializer.ContentPage, error) {
	db, err := dao.NewContentDao(ctx)
	if err != nil {
		return nil, err
	}
	question, err := db.FindByID(questionID)
	if dao.IsNotFound(err) {
		return nil, ErrContentNotFound
	}
	if err != nil {
		return nil, err
	}
	if question.Kind != model.ContentKindQuestion {
		return nil, ErrInvalidParent
	}
	return s.listByParent(ctx, db, questionID, model.ContentKindAnswer, page, limit)
}

// ListUserContents returns public posts for a user, with an optional kind filter.
func (s *ContentSrv) ListUserContents(ctx context.Context, userID uint, kind string, page, limit int) (*serializer.ContentPage, error) {
	if kind != "" && kind != string(model.ContentKindQuestion) && kind != string(model.ContentKindAnswer) && kind != string(model.ContentKindArticle) {
		return nil, ErrInvalidKind
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	db, err := dao.NewContentDao(ctx)
	if err != nil {
		return nil, err
	}
	items, total, err := db.ListByAuthor(userID, model.ContentKind(kind), page, limit)
	if err != nil {
		return nil, err
	}
	return &serializer.ContentPage{Items: buildContentResponses(items), Total: total, Page: page, Limit: limit}, nil
}

// ListComments returns comments attached to a question, answer, or article.
func (s *ContentSrv) ListComments(ctx context.Context, contentID uint, page, limit int) (*serializer.ContentPage, error) {
	db, err := dao.NewContentDao(ctx)
	if err != nil {
		return nil, err
	}
	parent, err := db.FindByID(contentID)
	if dao.IsNotFound(err) {
		return nil, ErrContentNotFound
	}
	if err != nil {
		return nil, err
	}
	if parent.Kind == model.ContentKindComment {
		return nil, ErrInvalidParent
	}
	return s.listByParent(ctx, db, contentID, model.ContentKindComment, page, limit)
}

// CreateComment attaches a comment to a public content item and records the
// creation time. Comments are stored in contents but excluded from Feed candidates.
func (s *ContentSrv) CreateComment(ctx context.Context, authorID, parentID uint, req serializer.CreateCommentRequest) (*serializer.ContentResponse, error) {
	if authorID == 0 || parentID == 0 || strings.TrimSpace(req.Body) == "" {
		return nil, ErrInvalidInput
	}
	db, err := dao.NewContentDao(ctx)
	if err != nil {
		return nil, err
	}
	parent, err := db.FindByID(parentID)
	if dao.IsNotFound(err) {
		return nil, ErrContentNotFound
	}
	if err != nil {
		return nil, err
	}
	if parent.Kind == model.ContentKindComment {
		return nil, ErrInvalidParent
	}
	content := &model.Content{AuthorID: authorID, Kind: model.ContentKindComment, ParentID: &parentID, Topic: parent.Topic, Body: strings.TrimSpace(req.Body)}
	if err := db.Create(content); err != nil {
		return nil, err
	}
	created, err := db.FindByID(content.ID)
	if err != nil {
		return nil, err
	}
	response := buildContentResponse(created)
	return &response, nil
}

func (s *ContentSrv) listByParent(ctx context.Context, db *dao.ContentDao, parentID uint, kind model.ContentKind, page, limit int) (*serializer.ContentPage, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	items, total, err := db.ListByParent(parentID, kind, page, limit)
	if err != nil {
		return nil, err
	}
	return &serializer.ContentPage{Items: buildContentResponses(items), Total: total, Page: page, Limit: limit}, nil
}

func buildContentResponses(items []model.Content) []serializer.ContentResponse {
	responses := make([]serializer.ContentResponse, 0, len(items))
	for i := range items {
		responses = append(responses, buildContentResponse(&items[i]))
	}
	return responses
}

func buildContentResponse(content *model.Content) serializer.ContentResponse {
	return serializer.ContentResponse{
		ID: content.ID,
		Author: serializer.ContentAuthor{
			ID: content.Author.ID, Username: content.Author.Username, DisplayName: content.Author.DisplayName,
			Avatar: content.Author.Avatar, IsBigV: content.Author.FollowerCount >= feedConf().BigVThreshold,
		},
		Kind: string(content.Kind), Topic: content.Topic, ParentID: content.ParentID, Title: content.Title, Body: content.Body,
		CreatedAt: content.CreatedAt.Unix(), UpdatedAt: content.UpdatedAt.Unix(),
	}
}

// GetFanoutStats returns how the consumer delivered a content.created event.
// ok is false while the event is still in the outbox or the queue.
func (s *ContentSrv) GetFanoutStats(ctx context.Context, contentID uint) (serializer.FanoutStats, bool, error) {
	return cache.NewFeedCache().GetFanoutStats(ctx, contentID)
}
