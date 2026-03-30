package cmd

import (
	"embed"
	"image"
	"image/color"
	_ "image/png"
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// содержать виртуальную файловую систему с вашими файлами,
// "вшитыми" прямо внутрь исполняемого файла

//go:embed asset/image/*
var assetFS embed.FS

const (
	DeadZoneW = 150  // Ширина "свободной зоны" в пикселях
	WorldW    = 1000 // Общая длина твоего уровня

	ScreenW, ScreenH = 640, 360
	PlayerW, PlayerH = 32, 32

	gravity     = 0.25 // Сила гравитации (насколько быстро разгоняется вниз)
	jumpSpeed   = -5.5 // Сила прыжка (отрицательная, так как Y идет вверх)
	runSpeed    = 0.3  // Ускорение бега влево/вправо
	maxRunSpeed = 3.0  // Максимальная скорость бега
	friction    = 1.0  // Сила трения
)

// Структура нашего цветного блока (платформы)
type Block struct {
	rect  image.Rectangle // Позиция и размер
	color color.RGBA      // Цвет
}

// структура хранящая все необходимые данные для работы игры
type Game struct {
	// цвет фона
	backgroundColor color.RGBA

	// картинка игрока
	playerImg *ebiten.Image

	// координаты камеры
	cameraX, cameraY float64

	// Массив всех блоков на уровне
	blocks []Block

	jumpBufferTimer int // Сколько кадров еще действует нажатие прыжка

	lastDir float64 // Какое направление было нажато последним (-1 или 1)

	// скорость игрока
	playerVX float64 // Скорость по оси X (для бега)
	playerVY float64 // Скорость по оси Y (для падения и прыжка)

	// Находится ли персонаж на земле
	onGround bool

	// позиция игрока
	playerX, playerY float64

	// аттребуты
	fullscreen  bool
	initialized bool
}

// функция для загрузки изображения.
// здесь мы извлекаем файл из виртуальной файловой системы и декодируем его
func loadImage(assetPath string) *ebiten.Image {
	f, err := assetFS.Open(assetPath)
	if err != nil {
		log.Panic(err)
	}

	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		log.Panic(err)
	}

	// передаем данные
	return ebiten.NewImageFromImage(img)
}

// метод ресет который будет вызываться при запуске игры и всякий раз
// когда игрок будет выходить за пределы поля
func (g *Game) reset() {
	g.playerX = ScreenW/2 - PlayerW/2
	g.playerY = ScreenH/2 - PlayerH/2
	g.playerVX = 0
	g.playerVY = 0
}

// метод, который будет вызываться один раз при запуске игры
// здесь мы зададим начальное состояние игры и загрузим все ресурсы
func (g *Game) initialize() {
	g.backgroundColor = color.RGBA{0, 181, 226, 255}
	g.playerImg = loadImage("asset/image/knight.png")
	g.reset()

	g.fullscreen = true
	g.initialized = true

	g.blocks = []Block{
		{rect: image.Rect(0, 320, 1000, 360), color: color.RGBA{100, 100, 100, 255}}, // Пол
		{rect: image.Rect(400, 240, 500, 260), color: color.RGBA{255, 50, 50, 255}},  // Платформа 1
		{rect: image.Rect(300, 275, 400, 295), color: color.RGBA{50, 255, 50, 255}},  // Платформа 2
		{rect: image.Rect(530, 200, 550, 320), color: color.RGBA{50, 50, 255, 255}},  // Стена
	}

	g.initialized = true
}

// вызывается почти в каждом кадре для определения размера холста
// получает информацию о текущем размере окна приложения или о размере который задал пользователь
// возвращает размер области экнана
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return ScreenW, ScreenH
}

// Вспомогательная функция для проверки столкновений с использованием float64
func isColliding(px, py float64, b image.Rectangle) bool {
	return px < float64(b.Max.X) &&
		px+PlayerW > float64(b.Min.X) &&
		py < float64(b.Max.Y) &&
		py+PlayerH > float64(b.Min.Y)
}

// вызывается при каждом тике для обновления состояния игры.
// по умолчанию вызывается 60 раз в секунду частота тиков не зависит от частоты кадров
// и не меняется от зименения частоты кадров
// можно управлять частотой тиков при помощи SetTPS
func (g *Game) Update() error {
	if !g.initialized {
		g.initialize()
	}

	// проверяем были ли нажата клавиша F11, если да переключаем полноэкранный режим
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		g.fullscreen = !g.fullscreen
		ebiten.SetFullscreen(g.fullscreen)
	}

	// 1. ОБРАБОТКА ВВОДА ДЛЯ ДВИЖЕНИЯ (Last Input Wins)
	leftPressed := ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA)
	rightPressed := ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD)

	// Проверяем, какая клавиша была нажата именно в этом кадре
	leftJustPressed := inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA)
	rightJustPressed := inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeyD)

	if leftJustPressed {
		g.lastDir = -1
	} else if rightJustPressed {
		g.lastDir = 1
	}

	targetDir := 0.0
	if leftPressed && rightPressed {
		// Если зажаты обе — выбираем ту, что нажата последней
		targetDir = g.lastDir
	} else if leftPressed {
		targetDir = -1
	} else if rightPressed {
		targetDir = 1
	}

	// 2. БУФЕР ПРЫЖКА (Jump Buffer)
	// Если игрок нажал пробел — выставляем таймер (напр. 15 кадров)
	// При твоем TPS 100, 15 кадров — это 0.15 сек.
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.jumpBufferTimer = 15
	}

	// 3. ГОРИЗОНТАЛЬНАЯ ФИЗИКА (Разгон / Трение)
	if targetDir != 0 {
		g.playerVX += targetDir * runSpeed
	} else {
		friction := 0.2 // Чуть увеличил для отзывчивости
		if g.playerVX > 0 {
			g.playerVX = math.Max(0, g.playerVX-friction)
		} else if g.playerVX < 0 {
			g.playerVX = math.Min(0, g.playerVX+friction)
		}
	}

	// Ограничение скорости
	if g.playerVX > maxRunSpeed {
		g.playerVX = maxRunSpeed
	}
	if g.playerVX < -maxRunSpeed {
		g.playerVX = -maxRunSpeed
	}

	// Применение X и коллизии (твой старый код)
	g.playerX += g.playerVX
	for _, b := range g.blocks {
		if isColliding(g.playerX, g.playerY, b.rect) {
			if g.playerVX > 0 {
				g.playerX = float64(b.rect.Min.X) - PlayerW
			} else if g.playerVX < 0 {
				g.playerX = float64(b.rect.Max.X)
			}
			g.playerVX = 0 // Обнуляем скорость при ударе о стену!
		}
	}

	// 4. ВЕРТИКАЛЬНАЯ ФИЗИКА
	g.playerVY += gravity
	g.playerY += g.playerVY
	g.onGround = false

	// Коллизии по Y
	for _, b := range g.blocks {
		if isColliding(g.playerX, g.playerY, b.rect) {
			if g.playerVY > 0 { // Приземление
				g.playerY = float64(b.rect.Min.Y) - PlayerH
				g.playerVY = 0
				g.onGround = true
			} else if g.playerVY < 0 { // Удар головой
				g.playerY = float64(b.rect.Max.Y)
				g.playerVY = 0
			}
		}
	}

	// ПРИМЕНЕНИЕ ПРЫЖКА (с учетом буфера)
	if g.onGround && g.jumpBufferTimer > 0 {
		g.playerVY = jumpSpeed
		g.onGround = false
		g.jumpBufferTimer = 0 // Использовали буфер — обнуляем
	}

	// Уменьшаем таймер буфера каждый кадр
	if g.jumpBufferTimer > 0 {
		g.jumpBufferTimer--
	}

	// ПРОВЕРКА ВЫЛЕТА ЗА ПРЕДЕЛЫ МИРА
	if g.playerY > ScreenH || g.playerX < -50 || g.playerX > 1000+50 {
		g.reset()
	}

	// РАСЧЕТ КАМЕРЫ

	// 1. Находим края мертвой зоны на экране прямо сейчас
	// Центр экрана минус половина ширины зоны
	deadZoneLeft := g.cameraX + (ScreenW/2 - DeadZoneW/2)
	deadZoneRight := deadZoneLeft + DeadZoneW

	// 2. Если игрок левее левого края зоны — толкаем камеру влево
	if g.playerX < deadZoneLeft {
		g.cameraX = g.playerX - (ScreenW/2 - DeadZoneW/2)
	}

	// 3. Если игрок правее правого края зоны — толкаем камеру вправо
	playerRight := g.playerX + PlayerW
	if playerRight > deadZoneRight {
		g.cameraX = playerRight - (ScreenW/2 + DeadZoneW/2)
	}

	// 4. ОГРАНИЧЕНИЕ (CLAMP) - Чтобы не видеть пустоту
	//if g.cameraX < 0 {
	//	g.cameraX = 0
	//}
	//maxCamX := float64(WorldW - ScreenW)
	//if g.cameraX > maxCamX {
	//	g.cameraX = maxCamX
	//}

	return nil
}

// вызывается в кажом кадре для отрисовки изображения на экране
// по умолчанию частота кадров не ограничена. так как вертикальная синхронизация отключена,
// ее можно включить с помощью SetVsyncEnabled, в этом слцчае приложение будет выполнять
// отрисовку только тогда, когда экран будт готов отобразить следующий кадр
func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(g.backgroundColor)

	// РИСУЕМ БЛОКИ
	for _, b := range g.blocks {
		vector.DrawFilledRect(
			screen,
			// Используем renderCamX вместо g.cameraX
			float32(float64(b.rect.Min.X)-g.cameraX),
			float32(float64(b.rect.Min.Y)-g.cameraY),
			float32(b.rect.Dx()),
			float32(b.rect.Dy()),
			b.color,
			false,
		)
	}

	// РИСУЕМ ИГРОКА
	op := &ebiten.DrawImageOptions{}
	// Здесь тоже используем округленную камеру
	op.GeoM.Translate(g.playerX-g.cameraX, g.playerY-g.cameraY)
	screen.DrawImage(g.playerImg, op)
}
