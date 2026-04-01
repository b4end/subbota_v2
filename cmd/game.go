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
	ScreenW, ScreenH = 2360, 1440
	SRR              = ScreenH / 360 // Коэффициент разрешения экрана (screen resolution ratio)
	PlayerW, PlayerH = 28 * SRR, 32 * SRR

	DeadZoneW = 150 * SRR  // Ширина "свободной зоны" в пикселях
	WorldW    = 1000 * SRR // Общая длина твоего уровня

	gravity     = 0.25 * SRR // Сила гравитации (насколько быстро разгоняется вниз)
	jumpSpeed   = -5.5 * SRR // Сила прыжка (отрицательная, так как Y идет вверх)
	runSpeed    = 0.3 * SRR  // Ускорение бега влево/вправо
	maxRunSpeed = 3.0 * SRR  // Максимальная скорость бега
	friction    = 2.0 * SRR  // Сила трения
)

// Структура нашего цветного блока (платформы)
type Block struct {
	rect  image.Rectangle // Позиция и размер
	color color.RGBA      // Цвет
}

type Sword struct {
	x, y        float64 // Текущие координаты меча
	angle       float64 // Текущий угол поворота меча
	targetAngle float64 // Угол, куда меч стремится
	img         *ebiten.Image
}

// структура хранящая все необходимые данные для работы игры
type Game struct {
	// цвет фона
	backgroundColor color.RGBA

	// картинка игрока
	playerImg *ebiten.Image

	sword Sword
	ticks int // Счетчик времени для анимации покачивания (idle)

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
	g.sword.x = ScreenW/2 - PlayerW/2
	g.sword.y = ScreenH/2 - PlayerH/2
	g.playerVX = 0
	g.playerVY = 0
}

// метод, который будет вызываться один раз при запуске игры
// здесь мы зададим начальное состояние игры и загрузим все ресурсы
func (g *Game) initialize() {
	g.backgroundColor = color.RGBA{0, 181, 226, 255}
	if ScreenH == 360 {
		g.playerImg = loadImage("asset/image/knight360.png")
		g.sword.img = loadImage("asset/image/sword360.png")
	} else if ScreenH == 720 {
		g.playerImg = loadImage("asset/image/knight720.png")
		g.sword.img = loadImage("asset/image/sword720.png")
	} else if ScreenH == 1440 {
		g.playerImg = loadImage("asset/image/knight1440.png")
		g.sword.img = loadImage("asset/image/sword1440.png")
	}

	g.reset()

	g.fullscreen = true
	g.initialized = true

	g.blocks = []Block{
		{rect: image.Rect(0*SRR, 320*SRR, 1000*SRR, 350*SRR), color: color.RGBA{100, 100, 100, 255}}, // Пол
		{rect: image.Rect(400*SRR, 240*SRR, 500*SRR, 260*SRR), color: color.RGBA{255, 50, 50, 255}},  // Платформа 1
		{rect: image.Rect(300*SRR, 275*SRR, 400*SRR, 295*SRR), color: color.RGBA{50, 255, 50, 255}},  // Платформа 2
		{rect: image.Rect(530*SRR, 200*SRR, 550*SRR, 320*SRR), color: color.RGBA{50, 50, 255, 255}},  // Стена
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
	if g.playerY > ScreenH || g.playerX < -50*SRR || g.playerX > (1000+50)*SRR {
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

	// ЛОГИКА МЕЧА (Встраиваем сюда)
	g.ticks++ // Увеличиваем счетчик для анимации idle

	// 1. ОПРЕДЕЛЯЕМ ЦЕЛЕВОЙ УГОЛ (где меч хочет быть)
	// По умолчанию меч "хочет" быть за спиной
	destAngle := 0.0
	if g.playerVX > 0 {
		destAngle = math.Pi * 0.85 // слева от игрока, если идем вправо
	} else if g.playerVX < 0 {
		destAngle = math.Pi*1.15 + math.Pi // справа от игрока, если идем влево
	} else {
		if g.lastDir == 1 {
			destAngle = math.Pi * 0.85
		} else {
			destAngle = math.Pi*1.15 + math.Pi
		}
	}

	// Небольшой наклон при прыжках/падении для динамики
	if g.playerVY > 0 {
		destAngle += 0.4
	} else if g.playerVY < 0 {
		destAngle -= 0.4
	}

	// 2. ПЛАВНЫЙ ПЕРЕЛЕТ УГЛА (Lerp Angle)
	// Ищем кратчайший путь между углами, чтобы меч не крутил лишние круги
	const angleSpeed = 0.1
	diff := destAngle - g.sword.targetAngle
	for diff < -math.Pi {
		diff += 2 * math.Pi
	}
	for diff > math.Pi {
		diff -= 2 * math.Pi
	}
	g.sword.targetAngle += diff * angleSpeed

	// 3. РАСЧЕТ ДИНАМИЧЕСКОГО РАДИУСА (Сплющивание)
	baseRadius := 18.0 * SRR
	// Легкое покачивание (idle)
	//baseRadius += math.Sin(float64(g.ticks)*0.06) * 1.5

	// Эффект "выталкивания" из верхней и нижней зон
	// Когда Sin(angle) близок к 1 или -1 (верх/низ), уменьшаем радиус (вдавливаем к игроку)
	pinch := math.Abs(math.Sin(g.sword.targetAngle))
	currentRadius := baseRadius * (1.0 - pinch*0.5) // Вдавливание на 35%

	// 4. ЦЕЛЕВЫЕ КООРДИНАТЫ (вокруг центра игрока)
	centerX := g.playerX + PlayerW/2
	centerY := g.playerY + PlayerH/2

	targetX := centerX + math.Cos(g.sword.targetAngle)*currentRadius
	targetY := centerY + math.Sin(g.sword.targetAngle)*currentRadius

	// 5. ВЯЗКАЯ ФИЗИКА (Следование меча за целью)
	// Чем меньше число, тем более "ленивым" и плавным будет меч (как в Soul Knight)
	const followSpeed = 0.7
	g.sword.x += (targetX - g.sword.x) * followSpeed
	g.sword.y += (targetY - g.sword.y) * followSpeed / 3

	// 6. ПОВОРОТ САМОГО МЕЧА
	// Меч всегда направлен острием от центра игрока
	g.sword.angle = math.Atan2(g.sword.y-centerY, g.sword.x-centerX)

	return nil
}

// вызывается в кажом кадре для отрисовки изображения на экране
// по умолчанию частота кадров не ограничена. так как вертикальная синхронизация отключена,
// ее можно включить с помощью SetVsyncEnabled, в этом слцчае приложение будет выполнять
// отрисовку только тогда, когда экран будт готов отобразить следующий кадр
func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(g.backgroundColor)

	// 1. РИСУЕМ БЛОКИ
	for _, b := range g.blocks {
		vector.DrawFilledRect(
			screen,
			float32(float64(b.rect.Min.X)-g.cameraX),
			float32(float64(b.rect.Min.Y)-g.cameraY),
			float32(b.rect.Dx()),
			float32(b.rect.Dy()),
			b.color,
			false,
		)
	}

	// Общий офсет камеры для игрока и меча
	offX := math.Floor(g.cameraX)
	offY := math.Floor(g.cameraY)

	// 2. РИСУЕМ ИГРОКА
	op := &ebiten.DrawImageOptions{}

	// ПОВОРОТ ИГРОКА (ОТЗЕРКАЛИВАНИЕ)
	if g.playerVX < 0 {
		op.GeoM.Scale(-1, 1)
		op.GeoM.Translate(float64(PlayerW), 0)
	} else if g.playerVX == 0 {
		if g.lastDir == -1 {
			op.GeoM.Scale(-1, 1)
			op.GeoM.Translate(float64(PlayerW), 0)
		}
	}

	drawX := math.Floor(g.playerX - offX)
	drawY := math.Floor(g.playerY - offY)
	op.GeoM.Translate(drawX, drawY)
	screen.DrawImage(g.playerImg, op)

	// 3. РИСУЕМ МЕЧ (Встраиваем логику сюда)
	if g.sword.img != nil {
		swordOp := &ebiten.DrawImageOptions{}

		// А. Центрируем спрайт меча, чтобы он вращался вокруг своего центра/рукояти
		sw, sh := g.sword.img.Bounds().Dx(), g.sword.img.Bounds().Dy()
		swordOp.GeoM.Translate(-float64(sw)/2, -float64(sh)/2)

		// Б. Применяем угол поворота (вычисленный в Update)
		swordOp.GeoM.Rotate(g.sword.angle)

		// В. Переносим в мировые координаты с учетом камеры
		// Используем те же offX и offY, что и для игрока
		swordDrawX := g.sword.x - offX
		swordDrawY := g.sword.y - offY

		// Округляем позицию меча, чтобы он не дрожал относительно игрока
		swordOp.GeoM.Translate(math.Floor(swordDrawX), math.Floor(swordDrawY))

		screen.DrawImage(g.sword.img, swordOp)
	}
}
