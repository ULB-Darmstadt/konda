# Running KONDA with docker compose

## 1. Create and edit .env file 
```
cd docker
cp ../.env.example .env
# edit .env as needed
```
## 2. Get BaseDB from LFS
```
git lfs pull
```

## 3. Start KONDA
```
docker compose up -d
```

### Updating running docker services
```
git pull
docker compose up -d --build --force-recreate
```