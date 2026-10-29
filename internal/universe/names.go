package universe

import (
	"fmt"
)

// Naming is independent of topology/geology. Only generated designations change;
// custom names and every system/object ID remain intact.
func DiversifySystemNames(g *Galaxy) bool {
	if g.NamingVersion >= 1 {
		return false
	}
	first := []string{"Тихая", "Дальняя", "Светлая", "Янтарная", "Северная", "Новая", "Серебряная", "Лазурная", "Звёздная", "Вольная", "Скрытая", "Золотая", "Вечерняя", "Утренняя", "Полярная", "Синяя", "Багряная", "Белая", "Стальная", "Вечная"}
	second := []string{"Гавань", "Аврора", "Колыбель", "Искра", "Обитель", "Заря", "Вершина", "Лагуна", "Корона", "Долина", "Тишина", "Грань", "Стезя", "Надежда", "Окраина", "Цитадель", "Вуаль", "Мечта", "Орбита", "Река", "Заводь", "Дюна", "Твердыня", "Дуга", "Сфера", "Песнь", "Мгла", "Роща", "Чаша", "Тропа", "Лира", "Струна", "Спираль", "Арка", "Жемчужина", "Мозаика", "Волна", "Стрела", "Скала", "Высота"}
	for i := range g.Stars {
		s := &g.Stars[i]
		old := s.Name
		legacy := fmt.Sprintf("ED-%04d", s.ID)
		if s.ID > 1000 {
			legacy = fmt.Sprintf("NX-%04d", s.ID-1000)
		}
		if old != legacy {
			continue
		}
		switch s.ID % 5 {
		case 0: // Retain a recognizable fraction of ED/NX designations.
		case 1:
			s.Name = fmt.Sprintf("KE-%04d", s.ID)
		case 2:
			s.Name = fmt.Sprintf("AX-%04d", s.ID)
		default:
			n := (s.ID-1)/5*2 + s.ID%5 - 3
			s.Name = first[n%len(first)] + " " + second[(n/len(first))%len(second)]
			if n >= len(first)*len(second) {
				s.Name += fmt.Sprintf(" %d", n/(len(first)*len(second))+1)
			}
		}
		for j := range s.Planets {
			if s.Planets[j].Name == fmt.Sprintf("%s-%c", old, 'b'+j) {
				s.Planets[j].Name = fmt.Sprintf("%s-%c", s.Name, 'b'+j)
			}
		}
	}
	g.NamingVersion = 1
	return true
}
