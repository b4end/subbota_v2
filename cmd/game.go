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

// embed.FS содержит виртуальную файловую систему с файлами,
// "вшитыми" прямо внутрь исполняемого файла

//go:embed asset/image/*
var assetFS embed.FS

const (
	ScreenW, ScreenH = 2360, 1440         // Разрешение экрана
	SRR              = ScreenH / 360      // Коэффициент разрешения экрана (screen resolution ratio)
	PlayerW, PlayerH = 28 * SRR, 32 * SRR // Размер игрока

	DeadZoneW = 150 * SRR  // Ширина "мертвой зоны" в пикселях
	WorldW    = 1000 * SRR // Общая длина уровня

	gravity     = 0.25 * SRR // Сила гравитации (насколько быстро разгоняется вниз)
	jumpSpeed   = -5.5 * SRR // Сила прыжка (отрицательная, так как Y идет вверх)
	runSpeed    = 0.3 * SRR  // Ускорение бега влево/вправо
	maxRunSpeed = 3.0 * SRR  // Максимальная скорость бега
	friction    = 2.0 * SRR  // Сила трения (уменьшение скорости)
)

// Block - структура платформы.
type Block struct {
	rect  image.Rectangle // Позиция и размер
	color color.RGBA      // Цвет
}

// Sword - струкрура, хранящая данные о мече.
type Sword struct {
	x, y        float64       // Текущие координаты меча
	angle       float64       // Текущий угол поворота меча
	targetAngle float64       // Угол, куда меч стремится
	img         *ebiten.Image // Изображение меча
}

// Game - структура, хранящая все необходимые данные для работы игры.
type Game struct {
	backgroundColor color.RGBA // Цвет фона

	playerImg *ebiten.Image // Картинка игрока

	sword       Sword // Меч, структура Sword
	isAttacking bool  // Идет ли отака в данный момент
	attackTimer int   // Таймер атаки

	ticks int // Счетчик времени

	playerX, playerY float64 // Координаты игрока
	cameraX, cameraY float64 // Координаты камеры

	playerVX float64 // Скорость по оси X (для бега)
	playerVY float64 // Скорость по оси Y (для падения и прыжка)

	onGround bool // Находится ли персонаж на земле

	jumpBufferTimer int // Сколько кадров еще действует нажатие прыжка

	lastDir float64 // Какое направление было нажато последним (-1 или 1)
	lastVX  float64 // Последняя скорость

	blocks []Block // Массив всех блоков на уровне

	// аттребуты
	fullscreen  bool // Развернута ли игра во весь экран
	initialized bool // Инициализированна ли игра
}

// loadImage() - функция для загрузки изображения.
// Здесь мы извлекаем файл из виртуальной файловой системы и декодируем его.
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

	return ebiten.NewImageFromImage(img)
}

// reset() - метод структуры Game, который будет вызываться при запуске
// игры и всякий раз когда игрок будет выходить за пределы поля,
// в нем ты задаем стартовые данные.
func (g *Game) reset() {
	g.playerX = ScreenW/2 - PlayerW/2
	g.playerY = ScreenH/2 - PlayerH/2
	g.sword.x = ScreenW/2 - PlayerW/2
	g.sword.y = ScreenH/2 - PlayerH/2
	g.playerVX = 0
	g.playerVY = 0
	g.lastVX = 1
}

// initialize() - метод структуры Game, который будет вызываться один раз при запуске игры,
// здесь мы зададем начальное состояние игры и загружаем все ресурсы.
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

// Layout() - метод структуры Game, который вызывается почти в каждом кадре для определения размера холста,
// получает информацию о текущем размере окна приложения, или о размере который задал пользователь,
// возвращает размер области экнана.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return ScreenW, ScreenH
}

// isColliding() - вспомогательная функция для проверки столкновений с использованием float64
func isColliding(px, py float64, b image.Rectangle) bool {
	return px < float64(b.Max.X) &&
		px+PlayerW > float64(b.Min.X) &&
		py < float64(b.Max.Y) &&
		py+PlayerH > float64(b.Min.Y)
}

// Update() - функция, которая вызывается при каждом тике для обновления состояния игры.
func (g *Game) Update() error {
	// Если игра не иницаилизированна, инициализируем
	if !g.initialized {
		g.initialize()
	}

	// Проверяем были ли нажата клавиша F11, если да, переключаем полноэкранный режим
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		g.fullscreen = !g.fullscreen
		ebiten.SetFullscreen(g.fullscreen)
	}

	// ОБРАБОТКА ВВОДА ДЛЯ ДВИЖЕНИЯ

	leftPressed := ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA)
	rightPressed := ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD)

	// Проверяем, какая клавиша была нажата именно в этом кадре
	leftJustPressed := inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA)
	rightJustPressed := inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeyD)

	// Записываем плсдеднюю нажатую кнопку в g.lastDir
	if leftJustPressed {
		g.lastDir = -1
	} else if rightJustPressed {
		g.lastDir = 1
	}

	// Механика приоритета движения
	// Если во время нажатия одной клавищи нажимается другая,
	// отдается приоритет новому нажатию
	targetDir := 0.0
	if leftPressed && rightPressed {
		targetDir = g.lastDir
	} else if leftPressed {
		targetDir = -1
	} else if rightPressed {
		targetDir = 1
	}

	// БУФЕР ПРЫЖКА

	// Если игрок нажал пробел, выставляем таймер
	// Необходимо чтобы можно было нажимать пробел чуть заранее до касания земли
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.jumpBufferTimer = 20
	}

	// ГОРИЗОНТАЛЬНАЯ ФИЗИКА

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

	// Записываем последнюю не нулевую скорость в g.lastVX
	if g.playerVX != 0 {
		g.lastVX = g.playerVX
	}

	// Применение X и коллизии
	g.playerX += g.playerVX
	for _, b := range g.blocks {
		if isColliding(g.playerX, g.playerY, b.rect) {
			if g.playerVX > 0 {
				g.playerX = float64(b.rect.Min.X) - PlayerW
			} else if g.playerVX < 0 {
				g.playerX = float64(b.rect.Max.X)
			}
			g.playerVX = 0 // Обнуляем скорость при ударе о стену
		}
	}

	// ВЕРТИКАЛЬНАЯ ФИЗИКА

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

	// ПРИМЕНЕНИЕ ПРЫЖКА

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

	// Находим края мертвой зоны на экране прямо сейчас
	// Центр экрана минус половина ширины зоны
	deadZoneLeft := g.cameraX + (ScreenW/2 - DeadZoneW/2)
	deadZoneRight := deadZoneLeft + DeadZoneW

	// Если игрок левее левого края зоны — толкаем камеру влево
	if g.playerX < deadZoneLeft {
		g.cameraX = g.playerX - (ScreenW/2 - DeadZoneW/2)
	}

	// Если игрок правее правого края зоны — толкаем камеру вправо
	playerRight := g.playerX + PlayerW
	if playerRight > deadZoneRight {
		g.cameraX = playerRight - (ScreenW/2 + DeadZoneW/2)
	}

	// ОГРАНИЧЕНИЕ (CLAMP) - Чтобы не видеть пустоту
	// При необходимости раскомментировать

	//if g.cameraX < 0 {
	//	g.cameraX = 0
	//}
	//maxCamX := float64(WorldW - ScreenW)
	//if g.cameraX > maxCamX {
	//	g.cameraX = maxCamX
	//}

	// ЛОГИКА МЕЧА

	// Увеличиваем счетчик кадров (тиков) игры на 1
	g.ticks++

	// ОПРЕДЕЛЯЕМ ЦЕЛЕВОЙ УГОЛ (Идеальное положение, куда меч должен прийти)

	destAngle := 0.0
	if g.playerVX < 0 {
		destAngle = math.Pi * 0.15
	} else if g.playerVX > 0 {
		destAngle = math.Pi * 0.85
	} else {
		if g.lastVX < 0 {
			destAngle = math.Pi * 0.15
		} else {
			destAngle = math.Pi * 0.85
		}
	}

	// Добавляем реакцию на вертикальное движение (прыжок и падение)
	if g.playerVY > 0 {
		destAngle += 0.4
	} else if g.playerVY < 0 {
		destAngle -= 0.4
	}

	// ПЛАВНЫЙ ПЕРЕЛЕТ ЦЕЛЕВОГО УГЛА

	// (определяет, какую долю пути меча проходит за один кадр)
	const angleSpeed = 0.1

	// diff - это разница между тем, где мы ХОТИМ быть (destAngle),
	// и тем, куда наша виртуальная цель стремится СЕЙЧАС (g.sword.targetAngle).
	diff := destAngle - g.sword.targetAngle

	// НОРМАЛИЗАЦИЯ УГЛА

	// Эти циклы заставляют меч выбрать кратчайший путь
	for diff < -math.Pi {
		diff += 2 * math.Pi
	}
	for diff > math.Pi {
		diff -= 2 * math.Pi
	}

	// Сдвигаем текущий виртуальный угол на небольшой шаг в сторону идеального угла
	g.sword.targetAngle += diff * angleSpeed

	// ВЫЧИСЛЯЕМ "ИДЕАЛЬНУЮ" ЦЕЛЬ (Задаем базовый радиус орбиты меча)

	baseRadius := 18.0 * SRR

	// Находим точную координату X и Y центра спрайта игрока.
	centerX := g.playerX + PlayerW/2
	centerY := g.playerY + PlayerH/2

	// Конвертируем полярные координаты (радиус и угол) в привычные декартовы (X, Y)
	targetX := centerX + math.Cos(g.sword.targetAngle)*baseRadius
	targetY := centerY + math.Sin(g.sword.targetAngle)*baseRadius

	// ВЯЗКАЯ ФИЗИКА (Реальный меч пытается догнать виртуальную точку)

	// followSpeed - скорость, с которой физические координаты меча тянутся к целевой точке.
	const followSpeed = 0.2

	// Мы берем 20% от расстояния от меча до цели и прибавляем к текущей позиции
	g.sword.x += (targetX - g.sword.x) * followSpeed
	// То же самое, но делим followSpeed на 3 (около 0.066)
	g.sword.y += (targetY - g.sword.y) * followSpeed / 3

	// ВОЗВРАТ НА ОРБИТУ И ВЫТАЛКИВАНИЕ (Учитывая выталкивание)

	// math.Atan2 возвращает реальный угол между центром игрока и ТЕКУЩЕЙ физической позицией меча
	currentAngle := math.Atan2(g.sword.y-centerY, g.sword.x-centerX)

	// Высчитываем коэффициент выталкивания в зависимости от того, где сейчас меч.
	pinch := math.Abs(math.Sin(currentAngle))

	// Сплющиваем орбиту меча снизу, чтобы меч не сильно залезал за пол
	currentRadius := baseRadius
	if g.sword.angle < math.Pi && g.sword.angle > math.Pi*0 {
		currentRadius = baseRadius * (1.0 - pinch*0.5)
	}

	// Финальная позиция: Мы игнорируем сырые X и Y, которые получились в Шаге 4,
	// и жестко приравниваем их к новой вычисленной точке
	g.sword.x = centerX + math.Cos(currentAngle)*currentRadius
	g.sword.y = centerY + math.Sin(currentAngle)*currentRadius

	// ПОВОРОТ САМОГО МЕЧА (Визуальная ротация спрайта)

	// Записываем финальный угол в структуру меча.
	g.sword.angle = currentAngle

	return nil
}

// Draw() - метод структуры Game, вызывается в кажом кадре для отрисовки изображения на экране.
func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(g.backgroundColor)

	// РИСУЕМ БЛОКИ
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

	// РИСУЕМ ИГРОКА
	op := &ebiten.DrawImageOptions{}

	// ПОВОРОТ ИГРОКА (ОТЗЕРКАЛИВАНИЕ)
	if g.playerVX < 0 {
		op.GeoM.Scale(-1, 1)
		op.GeoM.Translate(float64(PlayerW), 0)
	} else if g.playerVX == 0 {
		if g.lastVX < 0 {
			op.GeoM.Scale(-1, 1)
			op.GeoM.Translate(float64(PlayerW), 0)
		}
	}

	drawX := math.Floor(g.playerX - offX)
	drawY := math.Floor(g.playerY - offY)
	op.GeoM.Translate(drawX, drawY)
	screen.DrawImage(g.playerImg, op)

	// РИСУЕМ МЕЧ
	if g.sword.img != nil {
		swordOp := &ebiten.DrawImageOptions{}

		// Центрируем спрайт меча, чтобы он вращался вокруг своего центра/рукояти
		sw, sh := g.sword.img.Bounds().Dx(), g.sword.img.Bounds().Dy()
		swordOp.GeoM.Translate(-float64(sw)/2, -float64(sh)/2)

		// Применяем угол поворота (вычисленный в Update)
		swordOp.GeoM.Rotate(g.sword.angle)

		// Переносим в мировые координаты с учетом камеры
		// Используем те же offX и offY, что и для игрока
		swordDrawX := g.sword.x - offX
		swordDrawY := g.sword.y - offY

		swordOp.GeoM.Translate(swordDrawX, swordDrawY) //math.Floor(swordDrawX), math.Floor(swordDrawY))

		screen.DrawImage(g.sword.img, swordOp)
	}
}
