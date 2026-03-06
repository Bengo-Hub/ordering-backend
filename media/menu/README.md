# Menu images for ordering-backend

This folder holds images referenced by seeded menu items. The seed script uses paths like `/images/menu/espresso.jpg`; for local or production file serving, you can place assets here and use `/media/menu/<filename>` (served at `GET /media/menu/*` when `ORDERING_MEDIA_ROOT` is set).

## Placeholder

- `placeholder-food.svg` – copied from cafe-website; use as fallback when a specific image is missing.

## Expected filenames (from seed)

Place real assets here if available; otherwise the app can use placeholders or leave `image_url` empty.

- espresso.jpg, cappuccino.jpg, icedlatte.jpeg  
- breakfast.jpg, breakfast2.jpeg, oats.jpeg  
- chocolate-lava-cake.jpg  
- burger.jpeg, burger1.jpeg, burger2.jpeg  
- salad .jpg, salad1.jpeg  

(Exact list is in `cmd/seed/main.go` under `menuItems`.)
