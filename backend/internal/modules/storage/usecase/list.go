package usecase

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

func (s *Service) List(ctx context.Context, actor model.Actor, in model.ListInput) (model.ListResult, error) {
	if in.Limit <= 0 {
		in.Limit = apiquery.DefaultLimit
	}
	if in.View == "" {
		in.View = "all"
	}
	prefix, err := normalizePrefix(in.Prefix)
	if err != nil {
		return model.ListResult{}, err
	}

	var items []model.Object
	switch in.View {
	case "starred":
		items, err = s.listStarred(ctx, actor)
	case "shared":
		items, err = s.listShared(ctx, actor)
	case "public":
		items, err = s.listPublic(ctx, actor)
	case "trash":
		items, err = s.listTrash(ctx, actor)
	case "recent":
		items, err = s.listRecent(ctx, actor)
	default:
		items, err = s.listPrefix(ctx, actor, prefix, in.Recursive || in.Q != "")
	}
	if err != nil {
		return model.ListResult{}, err
	}

	filtered := make([]model.Object, 0, len(items))
	q := strings.ToLower(strings.TrimSpace(in.Q))
	for i := range items {
		obj := items[i]
		if in.Kind != "" && obj.Kind != "folder" && obj.FileKind != in.Kind {
			continue
		}
		if in.Access != "" && obj.Access != in.Access {
			continue
		}
		if in.ModifiedFrom != "" && obj.UpdatedAt[:min(10, len(obj.UpdatedAt))] < in.ModifiedFrom {
			continue
		}
		if in.ModifiedTo != "" && obj.UpdatedAt[:min(10, len(obj.UpdatedAt))] > in.ModifiedTo {
			continue
		}
		if q != "" {
			blob := strings.ToLower(obj.Name + " " + obj.Key + " " + obj.MimeType)
			if !strings.Contains(blob, q) {
				continue
			}
		}
		filtered = append(filtered, obj)
	}
	sortObjects(filtered, in.Sort)
	page, total := apiquery.Slice(filtered, in.Limit, in.Offset)
	return model.ListResult{
		Items:  page,
		Total:  total,
		Limit:  in.Limit,
		Offset: in.Offset,
		Prefix: prefix,
		View:   in.View,
	}, nil
}

func (s *Service) listPrefix(ctx context.Context, actor model.Actor, prefix string, recursive bool) ([]model.Object, error) {
	delimiter := "/"
	if recursive {
		delimiter = ""
	}
	objects, prefixes, err := s.scan(ctx, prefix, delimiter, maxListScan)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	items := make([]model.Object, 0, len(objects)+len(prefixes))
	if !recursive {
		for _, p := range prefixes {
			if skipSystem(p.Prefix) {
				continue
			}
			obj := s.objectFromPrefix(p.Prefix)
			items = append(items, obj)
			seen[p.Prefix] = struct{}{}
		}
	}
	ptrs := make([]*model.Object, 0, len(objects))
	for _, info := range objects {
		if skipSystem(info.Key) {
			continue
		}
		if !recursive && info.Key == prefix {
			continue
		}
		folder := isFolderKey(info.Key)
		if folder && !recursive {
			if _, ok := seen[info.Key]; ok {
				continue
			}
		}
		obj := s.objectFromInfo(info, folder)
		items = append(items, obj)
	}
	for i := range items {
		ptrs = append(ptrs, &items[i])
	}
	s.enrich(ctx, actor, ptrs)
	return items, nil
}

func (s *Service) listStarred(ctx context.Context, actor model.Actor) ([]model.Object, error) {
	if s.q == nil || actor.UserID == 0 {
		return []model.Object{}, nil
	}
	rows, err := s.q.ListStorageStarsByUser(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row.ObjectKey)
	}
	return s.headKeys(ctx, actor, keys)
}

func (s *Service) listShared(ctx context.Context, actor model.Actor) ([]model.Object, error) {
	if s.q == nil || actor.UserID == 0 {
		return []model.Object{}, nil
	}
	rows, err := s.q.ListStorageSharesForUser(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row.ObjectKey)
	}
	return s.headKeys(ctx, actor, keys)
}

func (s *Service) listPublic(ctx context.Context, actor model.Actor) ([]model.Object, error) {
	if s.q == nil {
		return []model.Object{}, nil
	}
	rows, err := s.q.ListActivePublicLinks(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		if _, ok := seen[row.ObjectKey]; ok {
			continue
		}
		seen[row.ObjectKey] = struct{}{}
		keys = append(keys, row.ObjectKey)
	}
	return s.headKeys(ctx, actor, keys)
}

func (s *Service) listTrash(ctx context.Context, actor model.Actor) ([]model.Object, error) {
	if s.q == nil {
		return []model.Object{}, nil
	}
	rows, err := s.q.ListStorageTrash(ctx, db.ListStorageTrashParams{
		LimitCount: 200, OffsetCount: 0,
	})
	if err != nil {
		return nil, err
	}
	items := make([]model.Object, 0, len(rows))
	for _, row := range rows {
		obj := model.Object{
			ID:             row.OriginalKey,
			Name:           row.Name,
			Key:            row.OriginalKey,
			Prefix:         parentPrefix(row.OriginalKey),
			Bucket:         s.bucket(),
			Kind:           "file",
			FileKind:       classifyFileKind(row.Name, row.MimeType),
			Size:           row.SizeBytes,
			MimeType:       row.MimeType,
			Access:         "private",
			Metadata:       map[string]string{},
			CreatedAt:      formatTS(row.DeletedAt),
			UpdatedAt:      formatTS(row.DeletedAt),
			TrashUUID:      row.Uuid.String(),
			TrashExpiresAt: formatTS(row.ExpiresAt),
		}
		if isFolderKey(row.OriginalKey) {
			obj.Kind = "folder"
			obj.FileKind = "folder"
		}
		items = append(items, obj)
	}
	_ = actor
	return items, nil
}

func (s *Service) listRecent(ctx context.Context, actor model.Actor) ([]model.Object, error) {
	objects, _, err := s.scan(ctx, "", "", maxListScan)
	if err != nil {
		return nil, err
	}
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].LastModified.After(objects[j].LastModified)
	})
	if len(objects) > 50 {
		objects = objects[:50]
	}
	items := make([]model.Object, 0, len(objects))
	ptrs := make([]*model.Object, 0, len(objects))
	for _, info := range objects {
		if skipSystem(info.Key) || isFolderKey(info.Key) {
			continue
		}
		obj := s.objectFromInfo(info, false)
		items = append(items, obj)
	}
	for i := range items {
		ptrs = append(ptrs, &items[i])
	}
	s.enrich(ctx, actor, ptrs)
	return items, nil
}

func (s *Service) headKeys(ctx context.Context, actor model.Actor, keys []string) ([]model.Object, error) {
	items := make([]model.Object, 0, len(keys))
	ptrs := make([]*model.Object, 0, len(keys))
	for _, key := range keys {
		info, err := s.store.Head(ctx, key)
		if err != nil {
			continue
		}
		obj := s.objectFromInfo(info, isFolderKey(key))
		items = append(items, obj)
	}
	for i := range items {
		ptrs = append(ptrs, &items[i])
	}
	s.enrich(ctx, actor, ptrs)
	return items, nil
}

func (s *Service) Usage(ctx context.Context) (model.Usage, error) {
	s.usageMu.Lock()
	if time.Since(s.usageAt) < time.Minute && s.usage.TotalFiles+s.usage.TotalFolders > 0 {
		out := s.usage
		s.usageMu.Unlock()
		return out, nil
	}
	s.usageMu.Unlock()

	objects, prefixes, err := s.scan(ctx, "", "", maxListScan)
	if err != nil {
		return model.Usage{}, err
	}
	byKind := map[string]*model.UsageKind{}
	var used int64
	files := make([]model.Object, 0, len(objects))
	folderSet := map[string]struct{}{}
	for _, p := range prefixes {
		if !skipSystem(p.Prefix) {
			folderSet[p.Prefix] = struct{}{}
		}
	}
	for _, info := range objects {
		if skipSystem(info.Key) {
			continue
		}
		if isFolderKey(info.Key) {
			folderSet[info.Key] = struct{}{}
			continue
		}
		obj := s.objectFromInfo(info, false)
		used += obj.Size
		k := obj.FileKind
		if byKind[k] == nil {
			byKind[k] = &model.UsageKind{Kind: k}
		}
		byKind[k].Bytes += obj.Size
		byKind[k].Count++
		files = append(files, obj)
	}
	kinds := make([]model.UsageKind, 0, len(byKind))
	for _, v := range byKind {
		kinds = append(kinds, *v)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i].Bytes > kinds[j].Bytes })
	largest := append([]model.Object{}, files...)
	sort.Slice(largest, func(i, j int) bool { return largest[i].Size > largest[j].Size })
	if len(largest) > 8 {
		largest = largest[:8]
	}
	recent := append([]model.Object{}, files...)
	sort.Slice(recent, func(i, j int) bool { return recent[i].UpdatedAt > recent[j].UpdatedAt })
	if len(recent) > 8 {
		recent = recent[:8]
	}
	out := model.Usage{
		TotalBytes:     used,
		UsedBytes:      used,
		AvailableBytes: 0,
		QuotaBytes:     0,
		TotalFiles:     int64(len(files)),
		TotalFolders:   int64(len(folderSet)),
		ByKind:         kinds,
		Largest:        largest,
		Recent:         recent,
	}
	s.usageMu.Lock()
	s.usage = out
	s.usageAt = time.Now()
	s.usageMu.Unlock()
	return out, nil
}

func (s *Service) invalidateUsage() {
	s.usageMu.Lock()
	s.usageAt = time.Time{}
	s.usageMu.Unlock()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
