package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Announcement notifications (TEC-330). WhatsApp is intentionally not in the
// default channel set: announcements use in-app, e-mail and push.
const EventAnnouncementPublished = "announcements.published"

var AnnouncementChannels = []string{ChannelInapp, ChannelEmail, ChannelWebPush, ChannelExpoPush}

func announcementTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(msgtemplate.Locales)*len(AnnouncementChannels))
	for _, lang := range msgtemplate.Locales {
		for _, ch := range AnnouncementChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: "{{title}}", Body: "{{body}}", Format: "text",
			})
		}
	}
	return out
}

func init() {
	Register(Event{
		Code: EventAnnouncementPublished, Module: "announcements",
		DefaultChannels:  AnnouncementChannels,
		AudienceRoles:    []string{RoleDealer, RoleDistributor, RoleCenter},
		Placeholders:     []msgtemplate.Placeholder{ph("title", "Yeni duyuru", "New announcement"), ph("body", "Duyuru metni", "Announcement body")},
		UserConfigurable: true,
		Templates:        announcementTemplates(),
	})
}
