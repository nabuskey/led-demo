package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"os"
)

type dynamoDB struct {
	tableName string
	region string
	client  *dynamodb.DynamoDB
}

func (d *dynamoDB) New() {
	d.region = os.Getenv("AWS_DEFAULT_REGION")
	d.tableName = os.Getenv("TABLE_NAME")
	sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(d.region)}))
	d.client = dynamodb.New(sess)
}

func (d *dynamoDB) GetColorValue(name string) (string, error) {
	result, err := d.client.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(os.Getenv("TABLE_NAME")),
		Key: map[string]*dynamodb.AttributeValue{
			"name": {
				S: aws.String(name),
			},
		},
	})
	if err != nil {
		fmt.Println(fmt.Sprintf("dynamodb error: %v", err))
		return "", fmt.Errorf("DB error")
	}
	if result.Item == nil {
		fmt.Println("no item returned")
		return "", fmt.Errorf("no item")
	}
	color := Color{}
	err = dynamodbattribute.UnmarshalMap(result.Item, &color)
	if err != nil {
		fmt.Println(fmt.Sprintf("unmarshal error: %v", err))
		return "", fmt.Errorf("internal error")
	}
	fmt.Println(color.Value)
	return color.Value, nil
}

type Color struct {
	Name string
	Value string
}