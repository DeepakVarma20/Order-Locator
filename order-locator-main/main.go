package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"text/template"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Order struct {
	Name                   string `bson:"name"`
	Phone                  string `bson:"phone"`
	Address                string `bson:"address"`
	PreferableDeliveryTime string `bson:"preferable_delivery_time"`
}

var ordersCollection *mongo.Collection

func main() {

	clientOptions := options.Client().ApplyURI("mongodb+srv://?retryWrites=true&w=majority&appName=Cluster0")
	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.Background())

	err = client.Ping(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}

	ordersCollection = client.Database("order_db").Collection("orders")

	http.HandleFunc("/", formHandler)
	http.HandleFunc("/submit", submitHandler)
	http.HandleFunc("/map", mapHandler)
	http.HandleFunc("/api/orders", ordersAPIHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server listening on port %s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func formHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/form.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

func submitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	name := r.FormValue("name")
	phone := r.FormValue("phone")
	address := r.FormValue("address")
	preferableDeliveryTime := r.FormValue("preferable_delivery_time")

	order := Order{
		Name:                   name,
		Phone:                  phone,
		Address:                address,
		PreferableDeliveryTime: preferableDeliveryTime,
	}

	_, err = ordersCollection.InsertOne(context.Background(), order)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/map", http.StatusSeeOther)
}

func mapHandler(w http.ResponseWriter, r *http.Request) {

	cursor, err := ordersCollection.Find(context.Background(), bson.M{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cursor.Close(context.Background())

	var orders []Order
	for cursor.Next(context.Background()) {
		var order Order
		if err := cursor.Decode(&order); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		orders = append(orders, order)
	}
	if err := cursor.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var locations []map[string]float64
	for _, order := range orders {
		location, err := geocodeAddress(order.Address)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		locations = append(locations, location)
	}

	type PageData struct {
		Orders    []Order
		Locations []map[string]float64
	}
	data := PageData{
		Orders:    orders,
		Locations: locations,
	}

	tmpl, err := template.ParseFiles("templates/map.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func ordersAPIHandler(w http.ResponseWriter, r *http.Request) {

	cursor, err := ordersCollection.Find(context.Background(), bson.M{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cursor.Close(context.Background())

	var orders []Order
	for cursor.Next(context.Background()) {
		var order Order
		if err := cursor.Decode(&order); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		orders = append(orders, order)
	}
	if err := cursor.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var locations []map[string]float64
	for _, order := range orders {
		location, err := geocodeAddress(order.Address)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		locations = append(locations, location)
	}

	data := struct {
		Orders    []Order
		Locations []map[string]float64
	}{
		Orders:    orders,
		Locations: locations,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

func geocodeAddress(address string) (map[string]float64, error) {
	apiKey := "AIzaSyA1Rz_xGPNYMO7WyP1wYdVzVoMOCO_UUtQ"
	geocodeURL := fmt.Sprintf("https://maps.googleapis.com/maps/api/geocode/json?address=%s&key=%s", url.QueryEscape(address), apiKey)

	resp, err := http.Get(geocodeURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			Geometry struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Status != "OK" {
		return nil, fmt.Errorf("Geocode request failed with status: %s", result.Status)
	}

	location := map[string]float64{
		"lat": result.Results[0].Geometry.Location.Lat,
		"lng": result.Results[0].Geometry.Location.Lng,
	}
	return location, nil
}
