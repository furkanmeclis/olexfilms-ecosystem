-- name: GetAppSettings :one
SELECT * FROM app_settings WHERE id = 1;

-- name: UpdateAppSettings :one
UPDATE app_settings
SET primary_color = COALESCE(sqlc.narg(primary_color), primary_color),
    company_name = COALESCE(sqlc.narg(company_name), company_name),
    tagline = COALESCE(sqlc.narg(tagline), tagline),
    address = COALESCE(sqlc.narg(address), address),
    phone = COALESCE(sqlc.narg(phone), phone),
    email = COALESCE(sqlc.narg(email), email),
    website = COALESCE(sqlc.narg(website), website),
    footer_text = COALESCE(sqlc.narg(footer_text), footer_text),
    paper_size = COALESCE(sqlc.narg(paper_size), paper_size),
    logo_object_key = COALESCE(sqlc.narg(logo_object_key), logo_object_key)
WHERE id = 1
RETURNING *;

-- name: SetAppSettingsLogo :one
UPDATE app_settings
SET logo_object_key = sqlc.arg(logo_object_key)
WHERE id = 1
RETURNING *;

-- name: ClearAppSettingsLogo :one
UPDATE app_settings
SET logo_object_key = NULL
WHERE id = 1
RETURNING *;
